package comdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
)

// SyncVoiceModels синхронизирует каталог голосовых моделей провайдера
// (voice_models) для конкретного Kind (tts|stt|music|sts).
//
// В отличие от SyncProviderModels:
//   - не трогает gpt_models/realtime_models и user_gpt;
//   - не переназначает пользователей: выбранная голосовая модель хранится в
//     UniversalModelData.Voice и валидируется на стороне приложения (fallback).
func (d *DB) SyncVoiceModels(provider comdom.ProviderType, kind comdom.VoiceKind, modelNames []string) (comdom.ProviderModelsSyncResult, error) {
	result := comdom.ProviderModelsSyncResult{Provider: provider}
	if !provider.IsValid() {
		return result, fmt.Errorf("некорректный provider: %d", provider)
	}
	if !kind.IsValid() {
		return result, fmt.Errorf("некорректный kind: %s", kind)
	}

	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	normalized := make([]string, 0, len(modelNames))
	seen := make(map[string]struct{}, len(modelNames))
	for _, name := range modelNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("ошибка начала транзакции синхронизации голосовых моделей: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, name := range normalized {
		var existingID int64
		err := tx.QueryRowContext(ctx,
			`SELECT Id FROM voice_models WHERE Provider = ? AND Kind = ? AND Name = ? LIMIT 1`,
			provider, string(kind), name).Scan(&existingID)
		switch {
		case err == nil:
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO voice_models (Provider, Kind, IsDefault, Name) VALUES (?, ?, 0, ?)`,
				provider, string(kind), name); err != nil {
				return result, fmt.Errorf("ошибка сохранения голосовой модели %s: %w", name, err)
			}
		default:
			return result, fmt.Errorf("ошибка поиска голосовой модели %s: %w", name, err)
		}
		result.Synced++
	}

	// Удаляем устаревшие модели этого Kind.
	rows, err := tx.QueryContext(ctx,
		`SELECT Id, Name FROM voice_models WHERE Provider = ? AND Kind = ?`,
		provider, string(kind))
	if err != nil {
		return result, fmt.Errorf("ошибка получения текущего списка голосовых моделей: %w", err)
	}
	var staleIDs []int64
	var staleNames []string
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return result, fmt.Errorf("ошибка чтения голосовых моделей: %w", err)
		}
		if _, ok := seen[strings.TrimSpace(name)]; ok {
			continue
		}
		staleIDs = append(staleIDs, id)
		staleNames = append(staleNames, strings.TrimSpace(name))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return result, fmt.Errorf("ошибка итерации голосовых моделей: %w", err)
	}
	_ = rows.Close()

	for i, id := range staleIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM voice_models WHERE Id = ?`, id); err != nil {
			return result, fmt.Errorf("ошибка удаления голосовой модели %s: %w", staleNames[i], err)
		}
		result.Removed++
		result.RemovedNames = append(result.RemovedNames, staleNames[i])
	}

	modelRows, err := tx.QueryContext(ctx,
		`SELECT Id, Name, IsDefault FROM voice_models WHERE Provider = ? AND Kind = ? ORDER BY Name`,
		provider, string(kind))
	if err != nil {
		return result, fmt.Errorf("ошибка получения актуального списка голосовых моделей: %w", err)
	}
	for modelRows.Next() {
		var id uint64
		var name string
		var isDefault int8
		if err := modelRows.Scan(&id, &name, &isDefault); err != nil {
			_ = modelRows.Close()
			return result, fmt.Errorf("ошибка чтения актуальной голосовой модели: %w", err)
		}
		result.Models = append(result.Models, comdom.ProviderModel{
			ID:        id,
			Name:      strings.TrimSpace(name),
			Kind:      kind,
			IsDefault: isDefault == 1,
		})
	}
	if err := modelRows.Err(); err != nil {
		_ = modelRows.Close()
		return result, fmt.Errorf("ошибка итерации актуальных голосовых моделей: %w", err)
	}
	_ = modelRows.Close()

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("ошибка фиксации синхронизации голосовых моделей: %w", err)
	}

	return result, nil
}

// GetVoiceModels возвращает все голосовые модели провайдера (все Kind),
// отсортированные по виду и имени.
func (d *DB) GetVoiceModels(provider comdom.ProviderType) ([]comdom.ProviderModel, error) {
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	rows, err := d.conn.QueryContext(ctx,
		`SELECT Id, Name, Kind, IsDefault FROM voice_models WHERE Provider = ? ORDER BY Kind, Name`, provider)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения голосовых моделей: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var models []comdom.ProviderModel
	for rows.Next() {
		var id uint64
		var name, kind string
		var isDefault int8
		if err := rows.Scan(&id, &name, &kind, &isDefault); err != nil {
			return nil, fmt.Errorf("ошибка чтения голосовой модели: %w", err)
		}
		models = append(models, comdom.ProviderModel{
			ID:        id,
			Name:      strings.TrimSpace(name),
			Kind:      comdom.VoiceKind(kind),
			IsDefault: isDefault == 1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка итерации голосовых моделей: %w", err)
	}
	return models, nil
}

// GetProviderModels возвращает LLM-каталог провайдера из gpt_models/realtime_models.
func (d *DB) GetProviderModels(provider comdom.ProviderType, modelType comdom.ModelType) ([]comdom.ProviderModel, error) {
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	var table string
	switch {
	case modelType.IsGeneral():
		table = "gpt_models"
	case modelType.IsRealtime():
		table = "realtime_models"
	default:
		return nil, fmt.Errorf("некорректный тип модели: %d", modelType)
	}

	rows, err := d.conn.QueryContext(ctx, fmt.Sprintf(`SELECT Id, Name FROM %s WHERE Provider = ? ORDER BY Name`, table), provider)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения моделей провайдера: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var models []comdom.ProviderModel
	for rows.Next() {
		var id uint64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("ошибка чтения модели провайдера: %w", err)
		}
		models = append(models, comdom.ProviderModel{ID: id, Name: strings.TrimSpace(name)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка итерации моделей провайдера: %w", err)
	}
	return models, nil
}

// GetAnyUserAPIKey возвращает API-ключ любого пользователя для провайдера.
// Используется только для глобального обновления каталога моделей, когда
// каталог одинаков для всех пользователей. Порядок — по дате обновления.
func (d *DB) GetAnyUserAPIKey(provider comdom.ProviderType) (string, error) {
	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	var apiKey string
	err := d.conn.QueryRowContext(ctx,
		`SELECT ApiKey FROM user_api_keys
		  WHERE Provider IN (?, ?) AND ApiKey <> ''
		  ORDER BY UpdatedAt DESC
		  LIMIT 1`,
		provider.String(),
		fmt.Sprintf("%d", provider),
	).Scan(&apiKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("ошибка получения API-ключа: %w", err)
	}
	return strings.TrimSpace(apiKey), nil
}
