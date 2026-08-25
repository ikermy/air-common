package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ikermy/air-common/pkg/mode"
)

func (d *DB) UserTimeZone(userID uint32) (string, error) {
	if userID == 0 {
		return "", fmt.Errorf("получены некорректные данные: userID")
	}

	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	var tz sql.NullString
	err := d.conn.QueryRowContext(ctx, "SELECT TimeZone FROM users WHERE Id = ?", userID).Scan(&tz)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return "", fmt.Errorf("тайм-аут (%d с) при получении часового пояса пользователя: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return "", fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return "", fmt.Errorf("пользователь с ID %d не найден", userID)
		default:
			return "", fmt.Errorf("ошибка получения часового пояса пользователя: %w", err)
		}
	}

	if !tz.Valid {
		return "", fmt.Errorf("часовой пояс не установлен для пользователя %d", userID)
	}

	return tz.String, nil
}

func (d *DB) UserLanguage(userID uint32) string {
	if userID == 0 {
		return "en"
	}

	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	var ln sql.NullString
	err := d.conn.QueryRowContext(ctx, "SELECT UserLang(?)", userID).Scan(&ln)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return "en"
		case errors.Is(err, context.Canceled):
			return "en"
		case errors.Is(err, sql.ErrNoRows):
			return "en"
		default:
			return "en"
		}
	}

	if !ln.Valid {
		return "en"
	}

	return ln.String
}

func (d *DB) SetUserSubscriptionNotified(user uint32) error {
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	query := "UPDATE subscriptions SET Notified = TRUE WHERE UserId = ?"

	_, err := d.Conn().ExecContext(ctx, query, user)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при обновлении статуса уведомления: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка при обновлении статуса уведомления: %w", err)
		}
	}

	return nil
}

// GetNotificationChannel получает данные каналов уведомлений пользователя
func (d *DB) GetNotificationChannel(userID uint32) (json.RawMessage, error) {
	if userID == 0 {
		return nil, fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	var data sql.NullString
	if err := d.Conn().QueryRowContext(ctx, "SELECT GetNotificationChannel(?)", userID).Scan(&data); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при вызове функции GetNotificationChannel: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("каналы уведомлений не найдены")
		default:
			return nil, fmt.Errorf("ошибка вызова хранимой функции GetNotificationChannel: %w", err)
		}
	}

	if !data.Valid {
		return nil, fmt.Errorf("получены пустые данные")
	}

	return json.RawMessage(data.String), nil
}

// GetUserSubscriptionLimites получает лимиты подписки пользователя
func (d *DB) GetUserSubscriptionLimites(userID uint32) (json.RawMessage, error) {
	if userID == 0 {
		return nil, fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	var data sql.NullString
	if err := d.Conn().QueryRowContext(ctx, "SELECT GetUserSubscriptionLimites(?)", userID).Scan(&data); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при вызове функции GetUserSubscriptionLimites: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("данные подписки не найдены")
		default:
			return nil, fmt.Errorf("ошибка вызова хранимой функции GetUserSubscriptionLimites: %w", err)
		}
	}

	if !data.Valid {
		return nil, fmt.Errorf("получены пустые данные")
	}

	return json.RawMessage(data.String), nil
}

// DisableAllUserChannel отключает все каналы пользователя
func (d *DB) DisableAllUserChannel(userID uint32) error {
	if userID == 0 {
		return fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	if _, err := d.Conn().ExecContext(ctx, "CALL DisableAllUserChannel(?)", userID); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при отключении каналов: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка отключения каналов: %w", err)
		}
	}

	return nil
}

// SetChannelEnabled включает или отключает канал пользователя
func (d *DB) SetChannelEnabled(userID uint32, chName string, status bool) error {
	if userID == 0 || chName == "" {
		return fmt.Errorf("получены некорректные значения: userID или chName пусты")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	if _, err := d.Conn().ExecContext(ctx, "CALL SetChannelEnabled(?,?,?)", userID, chName, status); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при сохранении статуса канала: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка сохранения статуса канала: %w", err)
		}
	}

	return nil
}
