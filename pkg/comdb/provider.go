package comdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/crypto"
)

// GetActiveProvider получает тип провайдера активной модели пользователя без создания дочернего контекста дял максимальной производительности
func (d *DB) GetActiveProvider(userID uint32) (comdom.ProviderType, error) {
	if userID == 0 {
		return 0, fmt.Errorf("получен некорректный userID")
	}

	// Используем родительский контекст напрямую для максимальной производительности
	// Запрашиваем активные модели с лимитом 2, чтобы проверить уникальность за один запрос
	query := `SELECT Provider FROM user_models WHERE userID = ? AND IsActive = 1 LIMIT 2`
	rows, err := d.Conn().QueryContext(d.Context(), query, userID)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return 0, fmt.Errorf("тайм-аут при получении активной модели: %w", err)
		case errors.Is(err, context.Canceled):
			return 0, fmt.Errorf("операция отменена: %w", err)
		default:
			return 0, fmt.Errorf("GetActiveProvider: query error: %w", err)
		}
	}
	defer rows.Close()

	var providers []uint8
	for rows.Next() {
		var p uint8
		if err := rows.Scan(&p); err != nil {
			return 0, fmt.Errorf("GetActiveProvider: scan error: %w", err)
		}
		providers = append(providers, p)
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("GetActiveProvider: rows iteration error: %w", err)
	}

	if len(providers) == 0 {
		return 0, fmt.Errorf("GetActiveProvider: %w", fmt.Errorf("активная модель не найдена"))
	}

	if len(providers) > 1 {
		return 0, fmt.Errorf("найдено несколько активных моделей (найдено %d)", len(providers))
	}

	return comdom.ProviderType(providers[0]), nil
}

// GetUserApiKey возвращает API-ключ пользователя для провайдера.
// Канонический формат Provider в user_api_keys — строковый ("google").
// Numeric provider ("3") читается только для обратной совместимости.
// Автоматически расшифровывает "$app$"; "$mk$" расшифровывает при наличии
// MasterKeyResolver, иначе оставляет для внешней model.WithMasterKeyProvider обёртки.
func (d *DB) GetUserAPIKey(userID uint32, provider comdom.ProviderType) (string, error) {
	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	var apiKey string
	err := d.conn.QueryRowContext(ctx,
		`SELECT ApiKey
		   FROM user_api_keys
		  WHERE UserId = ? AND Provider IN (?, ?)
		  ORDER BY CASE WHEN Provider = ? THEN 0 ELSE 1 END
		  LIMIT 1`,
		userID,
		provider.String(),
		strconv.Itoa(int(provider)),
		provider.String(),
	).Scan(&apiKey)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil // ключ не найден — не ошибка
		}
		return "", fmt.Errorf("ошибка получения API-ключа: %w", err)
	}

	apiKey = strings.TrimSpace(apiKey)

	if crypto.IsEncryptedWithMasterKey(apiKey) {
		if d.MasterKeyResolver == nil {
			return apiKey, nil
		}
		masterKey, ok := d.MasterKeyResolver(userID)
		if !ok {
			return "", fmt.Errorf("API-ключ зашифрован MasterKey, но MasterKey пользователя %d не загружен", userID)
		}
		decrypted, err := crypto.DecryptFieldWithMasterKey(masterKey, apiKey)
		if err != nil {
			return "", fmt.Errorf("ошибка расшифровки API-ключа через MasterKey: %w", err)
		}
		return strings.TrimSpace(decrypted), nil
	}

	// Если ключ не зашифрован — возвращаем как есть (backward compatibility)
	if !crypto.IsEncryptedWithAppKey(apiKey) {
		return apiKey, nil
	}

	// Расшифровываем через global encryptor
	encryptor, err := crypto.GetGlobalEncryptor()
	if err != nil {
		return "", fmt.Errorf("application encryption key не доступен: %w", err)
	}

	decrypted, err := encryptor.DecryptField(apiKey)
	if err != nil {
		return "", fmt.Errorf("ошибка расшифровки API-ключа: %w", err)
	}

	return strings.TrimSpace(decrypted), nil
}

// SetUserAPIKey сохраняет API-ключ пользователя с автоматическим шифрованием,
// если MasterKey пользователя загружен или application encryption key установлен.
func (d *DB) SetUserAPIKey(userID uint32, provider comdom.ProviderType, apiKey string) error {
	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	keyToStore := strings.TrimSpace(apiKey)
	if keyToStore == "" {
		return fmt.Errorf("API-ключ не может быть пустым")
	}

	if d.MasterKeyResolver != nil {
		if masterKey, ok := d.MasterKeyResolver(userID); ok {
			encrypted, encErr := crypto.EncryptFieldWithMasterKey(masterKey, keyToStore)
			if encErr != nil {
				return fmt.Errorf("ошибка шифрования API-ключа через MasterKey: %w", encErr)
			}
			keyToStore = encrypted
		}
	}

	// Пытаемся зашифровать если application encryption key доступен
	if !crypto.IsEncryptedWithMasterKey(keyToStore) {
		encryptor, err := crypto.GetGlobalEncryptor()
		if err == nil && encryptor.IsKeySet() {
			encrypted, encErr := encryptor.EncryptField(keyToStore)
			if encErr != nil {
				return fmt.Errorf("ошибка шифрования API-ключа application key: %w", encErr)
			}
			keyToStore = encrypted
		}
	}

	// Сохраняем (INSERT или UPDATE)
	query := `INSERT INTO user_api_keys (UserId, Provider, ApiKey)
	          VALUES (?, ?, ?)
	          ON DUPLICATE KEY UPDATE ApiKey = VALUES(ApiKey)`

	_, execErr := d.conn.ExecContext(ctx, query, userID, provider.String(), keyToStore)
	if execErr != nil {
		return fmt.Errorf("ошибка сохранения API-ключа: %w", execErr)
	}

	if _, err := d.conn.ExecContext(ctx,
		`DELETE FROM user_api_keys WHERE UserId = ? AND Provider = ?`,
		userID,
		strconv.Itoa(int(provider)),
	); err != nil {
		return fmt.Errorf("ошибка удаления legacy API-ключа: %w", err)
	}

	return nil
}

// DeleteUserAPIKey удаляет персональный API-ключ пользователя для провайдера.
func (d *DB) DeleteUserAPIKey(userID uint32, provider comdom.ProviderType) error {
	if userID == 0 {
		return fmt.Errorf("некорректный userID")
	}

	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	_, err := d.conn.ExecContext(ctx,
		`DELETE FROM user_api_keys WHERE UserId = ? AND Provider IN (?, ?)`,
		userID,
		provider.String(),
		strconv.Itoa(int(provider)),
	)
	if err != nil {
		return fmt.Errorf("ошибка удаления API ключа пользователя %d (%s): %w", userID, provider, err)
	}
	return nil
}

// SyncProviderModels синхронизирует каталог моделей провайдера с уже полученным списком моделей в зависимости от типа модели.
// При удалении неподдерживаемой модели из провайдера она удаляется из
// gpt_models или realtime_models, а ссылка в user_gpt (Model или Realtime)
// переводится на модель по умолчанию. Связь user_models при этом сохраняется.
func (d *DB) SyncProviderModels(union comdom.Union, modelNames []string) (comdom.ProviderModelsSyncResult, error) {
	result := comdom.ProviderModelsSyncResult{Provider: union.Provider}
	if !union.Provider.IsValid() {
		return result, fmt.Errorf("некорректный provider: %d", union.Provider)
	}

	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	var tableName string
	var userModelColumn string
	switch {
	case union.ModelType.IsGeneral():
		tableName = "gpt_models"
		userModelColumn = "Model"
	case union.ModelType.IsRealtime():
		tableName = "realtime_models"
		userModelColumn = "Realtime"
	default:
		return result, fmt.Errorf("некорректный тип модели: %d", union.ModelType)
	}

	normalizedNames := make([]string, 0, len(modelNames))
	for _, name := range modelNames {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			normalizedNames = append(normalizedNames, trimmed)
		}
	}

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("ошибка начала транзакции синхронизации: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	seenNames := make(map[string]struct{}, len(normalizedNames))
	for _, name := range normalizedNames {
		if _, ok := seenNames[name]; ok {
			continue
		}
		seenNames[name] = struct{}{}

		var existingID int64
		err := tx.QueryRowContext(ctx, fmt.Sprintf(`
			SELECT Id
			FROM %s
			WHERE Provider = ? AND Name = ?
			LIMIT 1
		`, tableName), union.Provider, name).Scan(&existingID)
		switch {
		case err == nil:
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
				INSERT INTO %s (Provider, IsDefault, Name)
				VALUES (?, ?, ?)
			`, tableName), union.Provider, 0, name); err != nil {
				return result, fmt.Errorf("ошибка сохранения модели %s: %w", name, err)
			}
		default:
			return result, fmt.Errorf("ошибка поиска модели %s: %w", name, err)
		}
		result.Synced++
	}

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT Id, Name FROM %s WHERE Provider = ?`, tableName), union.Provider)
	if err != nil {
		return result, fmt.Errorf("ошибка получения текущего списка моделей: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]struct{}, len(normalizedNames))
	for _, name := range normalizedNames {
		seen[name] = struct{}{}
	}

	var staleModels []struct {
		id   int64
		name string
	}
	for rows.Next() {
		var modelID int64
		var modelName string
		if err := rows.Scan(&modelID, &modelName); err != nil {
			return result, fmt.Errorf("ошибка чтения текущего списка моделей: %w", err)
		}
		trimmedModelName := strings.TrimSpace(modelName)
		if _, ok := seen[trimmedModelName]; ok {
			continue
		}

		staleModels = append(staleModels, struct {
			id   int64
			name string
		}{id: modelID, name: trimmedModelName})
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("ошибка итерации текущего списка моделей: %w", err)
	}
	if err := rows.Close(); err != nil {
		return result, fmt.Errorf("ошибка закрытия списка моделей: %w", err)
	}

	for _, stale := range staleModels {
		var affectedUsers []uint32
		userRows, err := tx.QueryContext(ctx, fmt.Sprintf(`
			SELECT um.userID
			FROM user_models um
			JOIN user_gpt ug ON ug.Id = um.ModelId
			WHERE ug.Provider = ? AND ug.%s = ?
		`, userModelColumn), union.Provider, stale.id)
		if err != nil {
			return result, fmt.Errorf("ошибка получения пользователей, использующих удалённую модель %s: %w", stale.name, err)
		}
		for userRows.Next() {
			var userID uint32
			if err := userRows.Scan(&userID); err != nil {
				continue
			}
			affectedUsers = append(affectedUsers, userID)
		}
		_ = userRows.Close()

		// Старые специализированные записи (например, Mistral TTS/STT),
		// которые ранее ошибочно попали в realtime_models, могут не иметь
		// default-модели для замены. Если на такую запись никто не ссылается,
		// её можно безопасно удалить напрямую.
		if len(affectedUsers) == 0 {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE Id = ?`, tableName), stale.id); err != nil {
				return result, fmt.Errorf("ошибка удаления неиспользуемой модели %s из %s: %w", stale.name, tableName, err)
			}
			result.Removed++
			result.RemovedNames = append(result.RemovedNames, stale.name)
			continue
		}

		var replacementID int64
		err = tx.QueryRowContext(ctx, fmt.Sprintf(`
			SELECT Id FROM %s
			WHERE Provider = ? AND IsDefault = 1 AND Id <> ?
			LIMIT 1
		`, tableName), union.Provider, stale.id).Scan(&replacementID)
		if errors.Is(err, sql.ErrNoRows) {
			return result, fmt.Errorf("невозможно удалить модель %s: отсутствует модель по умолчанию для замены", stale.name)
		}
		if err != nil {
			return result, fmt.Errorf("ошибка поиска модели для замены %s: %w", stale.name, err)
		}

		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
			UPDATE user_gpt
			SET %s = ?, AssistantId = ''
			WHERE Provider = ? AND %s = ?
		`, userModelColumn, userModelColumn), replacementID, union.Provider, stale.id); err != nil {
			return result, fmt.Errorf("ошибка переназначения пользователей с модели %s: %w", stale.name, err)
		}

		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE Id = ?`, tableName), stale.id); err != nil {
			return result, fmt.Errorf("ошибка удаления модели %s из %s: %w", stale.name, tableName, err)
		}

		result.Removed++
		result.RemovedNames = append(result.RemovedNames, stale.name)
		result.ClearedUsers += len(affectedUsers)
		for _, userID := range affectedUsers {
			result.AffectedUsers = append(result.AffectedUsers, comdom.ProviderModelUserChange{
				UserID:    userID,
				ModelID:   uint64(stale.id),
				ModelName: stale.name,
			})
		}
	}

	// Читаем фактический список после синхронизации в рамках той же транзакции.
	modelRows, err := tx.QueryContext(ctx, fmt.Sprintf(
		`SELECT Id, Name FROM %s WHERE Provider = ? ORDER BY Name`, tableName), union.Provider)
	if err != nil {
		return result, fmt.Errorf("ошибка получения актуального списка моделей: %w", err)
	}
	for modelRows.Next() {
		var modelID uint64
		var modelName string
		if err := modelRows.Scan(&modelID, &modelName); err != nil {
			_ = modelRows.Close()
			return result, fmt.Errorf("ошибка чтения актуального списка моделей: %w", err)
		}
		trimmedName := strings.TrimSpace(modelName)
		result.Models = append(result.Models, comdom.ProviderModel{ID: modelID, Name: trimmedName})
	}
	if err := modelRows.Err(); err != nil {
		_ = modelRows.Close()
		return result, fmt.Errorf("ошибка итерации актуального списка моделей: %w", err)
	}
	if err := modelRows.Close(); err != nil {
		return result, fmt.Errorf("ошибка закрытия актуального списка моделей: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("ошибка фиксации синхронизации моделей: %w", err)
	}

	return result, nil
}
