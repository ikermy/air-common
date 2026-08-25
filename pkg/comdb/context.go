package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/mode"
)

// ReadContext читает контекст диалога из базы данных
func (d *DB) ReadContext(dialogId uint64, provider comdom.ProviderType) (json.RawMessage, error) {
	if dialogId == 0 {
		return nil, fmt.Errorf("получен пустой dialogId")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	var data sql.NullString
	if err := d.Conn().QueryRowContext(ctx, "SELECT ReadContext(?, ?)", dialogId, provider.String()).Scan(&data); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при вызове ReadContext: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена при вызове ReadContext: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("контекст диалога не найден")
		default:
			return nil, fmt.Errorf("ошибка вызова хранимой функции ReadContext: %w", err)
		}
	}

	if !data.Valid {
		return nil, fmt.Errorf("получены пустые данные")
	}

	return json.RawMessage(data.String), nil
}

// SaveContext сохраняет контекст диалога в базу данных
func (d *DB) SaveContext(threadId uint64, provider comdom.ProviderType, dialogContext json.RawMessage) error {
	if threadId == 0 {
		return fmt.Errorf("получен пустой тред")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	if _, err := d.Conn().ExecContext(ctx, "CALL SaveContext(?, ?, ?)", threadId, provider.String(), dialogContext); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при сохранении контекста: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка сохранения контекста: %w", err)
		}
	}

	return nil
}
