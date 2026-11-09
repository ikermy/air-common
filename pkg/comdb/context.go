package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/ikermy/air-common/v2/pkg/comdom"
	"github.com/ikermy/air-common/v2/pkg/mode"
)

// ReadContext читает контекст диалога из базы данных.
func (d *DB) ReadContext(dialogId uint64, provider comdom.ProviderType) (json.RawMessage, error) {
	if dialogId == 0 {
		return nil, fmt.Errorf("получен пустой dialogId")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	providerName := provider.String()

	if d.DialogV2ReadEnabled {
		var v2 sql.NullString
		err := d.Conn().QueryRowContext(ctx,
			"SELECT context_json FROM dialog_contexts WHERE dialog_id = ? AND provider = ?",
			dialogId, providerName).Scan(&v2)
		if err == nil && v2.Valid {
			return json.RawMessage(v2.String), nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("ошибка чтения контекста v2: %w", err)
		}
	}

	var data sql.NullString
	err := d.Conn().QueryRowContext(ctx,
		"SELECT JSON_EXTRACT(`Context`, CONCAT('$.', ?)) FROM dialogs WHERE Id = ?",
		providerName, dialogId).Scan(&data)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при чтении контекста: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена при чтении контекста: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("контекст диалога не найден")
		default:
			return nil, fmt.Errorf("ошибка чтения контекста: %w", err)
		}
	}

	if !data.Valid {
		return nil, fmt.Errorf("получены пустые данные")
	}

	if d.DialogV2ReadEnabled {
		if _, err := d.Conn().ExecContext(ctx,
			"INSERT IGNORE INTO dialog_contexts (dialog_id, user_id, provider, context_json) "+
				"SELECT Id, `User`, ?, ? FROM dialogs WHERE Id = ?",
			providerName, data.String, dialogId); err != nil {
			log.Printf("comdb: dialog_contexts backfill failed for dialog %d: %v", dialogId, err)
		}
	}

	return json.RawMessage(data.String), nil
}

// SaveContext сохраняет контекст диалога в базе данных.
func (d *DB) SaveContext(threadId uint64, provider comdom.ProviderType, dialogContext json.RawMessage) error {
	if threadId == 0 {
		return fmt.Errorf("получен пустой тред")
	}
	if len(dialogContext) == 0 {
		return fmt.Errorf("получен пустой контекст")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saveContext begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		userID     uint64
		currentCtx sql.NullString
	)
	if err := tx.QueryRowContext(ctx,
		"SELECT `User`, `Context` FROM dialogs WHERE Id = ? FOR UPDATE", threadId).
		Scan(&userID, &currentCtx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("диалог %d не найден", threadId)
		}
		return fmt.Errorf("saveContext read dialog: %w", err)
	}

	if !json.Valid(dialogContext) {
		return fmt.Errorf("контекст не является валидным JSON")
	}

	contexts := make(map[string]json.RawMessage)
	if currentCtx.Valid && currentCtx.String != "" {
		if err := json.Unmarshal([]byte(currentCtx.String), &contexts); err != nil {
			return fmt.Errorf("saveContext parse current context: %w", err)
		}
	}
	providerName := provider.String()
	contexts[providerName] = dialogContext

	newCtx, err := json.Marshal(contexts)
	if err != nil {
		return fmt.Errorf("saveContext marshal: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE dialogs SET `Context` = ?, `Date` = CURRENT_TIMESTAMP() WHERE Id = ?",
		string(newCtx), threadId); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при сохранении контекста: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка сохранения контекста: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dialog_contexts (dialog_id, user_id, provider, context_json)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE context_json = VALUES(context_json), updated_at = NOW(3)`,
		threadId, userID, providerName, string(dialogContext)); err != nil {
		return fmt.Errorf("ошибка сохранения контекста v2: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("saveContext commit: %w", err)
	}
	return nil
}
