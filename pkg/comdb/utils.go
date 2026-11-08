package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/mode"
)

func (d *DB) UserTimeZone(userID uint32) (string, error) {
	if userID == 0 {
		return "", fmt.Errorf("получены некорректные данные: userID")
	}

	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	conn, connErr := d.ConnForUser(ctx, userID)
	if connErr != nil {
		return "", fmt.Errorf("выбор шарда: %w", connErr)
	}

	var tz sql.NullString
	err := conn.QueryRowContext(ctx, "SELECT TimeZone FROM users WHERE Id = ?", userID).Scan(&tz)
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
	err := d.conn.QueryRowContext(ctx, `
		SELECT l.Code FROM users u
		JOIN languages l ON l.Id = u.Lang
		WHERE u.Id = ?`, userID).Scan(&ln)
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

	conn, connErr := d.ConnForUser(ctx, user)
	if connErr != nil {
		return fmt.Errorf("выбор шарда: %w", connErr)
	}

	_, err := conn.ExecContext(ctx, query, user)
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

	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("выбор шарда: %w", err)
	}

	var (
		email           bool
		telegramEnabled bool
		instant         bool
		telegram        sql.NullInt64
	)
	err = conn.QueryRowContext(ctx, `
		SELECT Email, Telegram_enabled, Instant, Telegram
		FROM notifications WHERE UserId = ?`, userID).
		Scan(&email, &telegramEnabled, &instant, &telegram)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при чтении каналов уведомлений: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		default:
			return nil, fmt.Errorf("ошибка чтения каналов уведомлений: %w", err)
		}
	}

	channels := make([]map[string]string, 0, 3)
	if email {
		var addr sql.NullString
		if err := conn.QueryRowContext(ctx, `
			SELECT Email FROM user_auth
			WHERE UserId = ? AND Confirmed = 1 AND Disabled = 0
			LIMIT 1`, userID).Scan(&addr); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("ошибка чтения email: %w", err)
		}
		value := "null"
		if addr.Valid {
			value = addr.String
		}
		channels = append(channels, map[string]string{"channel_type": "mail", "channel_value": value})
	}
	if telegramEnabled {
		value := "null"
		if telegram.Valid && telegram.Int64 != 0 {
			value = fmt.Sprintf("%d", telegram.Int64)
		}
		channels = append(channels, map[string]string{"channel_type": "telegram", "channel_value": value})
	}
	if instant {
		channels = append(channels, map[string]string{"channel_type": "instant", "channel_value": "1"})
	}

	result, err := json.Marshal(channels)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации каналов: %w", err)
	}
	return result, nil
}

// GetUserSubscriptionLimites получает лимиты подписки пользователя
func (d *DB) GetUserSubscriptionLimites(userID uint32) (json.RawMessage, error) {
	if userID == 0 {
		return nil, fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("выбор шарда: %w", err)
	}

	var (
		balance sql.NullFloat64
		endDate sql.NullTime
	)
	err = conn.QueryRowContext(ctx, `
		SELECT u.balance, s.EndDate
		FROM users u
		JOIN subscriptions s ON s.UserId = u.Id
		WHERE u.Id = ?`, userID).Scan(&balance, &endDate)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при чтении данных подписки: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("данные подписки не найдены")
		default:
			return nil, fmt.Errorf("ошибка чтения данных подписки: %w", err)
		}
	}

	result := map[string]any{"balance": nil, "EndDate": nil}
	if balance.Valid {
		result["balance"] = balance.Float64
	}
	if endDate.Valid {
		result["EndDate"] = endDate.Time.Format("2006-01-02")
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации данных подписки: %w", err)
	}
	return data, nil
}

// DisableAllUserChannel отключает все каналы пользователя
func (d *DB) DisableAllUserChannel(userID uint32) error {
	if userID == 0 {
		return fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("выбор шарда: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `
		UPDATE channels SET
			TgBot_enabled = 0,
			Widget_enabled = 0,
			TgUserBot_enabled = 0,
			Whats_enabled = 0,
			Insta_enabled = 0,
			Avito_enabled = 0
		WHERE UserId = ?`, userID); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при отключении каналов: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка отключения каналов: %w", err)
		}
	}

	if err := disableAllUserChannelV2(ctx, conn, userID); err != nil {
		return fmt.Errorf("ошибка отключения каналов v2: %w", err)
	}

	return nil
}

// disableAllUserChannelV2 отражает DisableAllUserChannel в user_channels.
func disableAllUserChannelV2(ctx context.Context, conn *sql.DB, userID uint32) error {
	_, err := conn.ExecContext(ctx,
		`UPDATE user_channels SET enabled = 0 WHERE user_id = ?`, userID)
	return err
}

// SetChannelEnabled включает или отключает канал пользователя
func (d *DB) SetChannelEnabled(userID uint32, chName string, status bool) error {
	if userID == 0 || chName == "" {
		return fmt.Errorf("получены некорректные значения: userID или chName пусты")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	ch, ok := comdom.ChannelByLegacyName(chName)
	if !ok || !ch.Configurable || ch.LegacyColumn == "" {
		return fmt.Errorf("неизвестный тип канала: %s", chName)
	}

	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("выбор шарда: %w", err)
	}

	if _, err := conn.ExecContext(ctx,
		fmt.Sprintf("UPDATE channels SET `%s` = ? WHERE UserId = ?", ch.LegacyColumn), status, userID); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при сохранении статуса канала: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка сохранения статуса канала: %w", err)
		}
	}

	if err := setChannelEnabledV2(ctx, conn, userID, ch.Slug, status); err != nil {
		return fmt.Errorf("ошибка сохранения статуса канала v2: %w", err)
	}

	return nil
}

// setChannelEnabledV2 mirrors SetChannelEnabled into user_channels.
func setChannelEnabledV2(ctx context.Context, conn *sql.DB, userID uint32, channelSlug string, status bool) error {
	ch, ok := comdom.ChannelBySlug(channelSlug)
	if !ok {
		return fmt.Errorf("неизвестный тип канала: %s", channelSlug)
	}
	_, err := conn.ExecContext(ctx, `
		INSERT INTO user_channels (id, user_id, channel_type, enabled)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE enabled = VALUES(enabled)`,
		uint64(userID)*10+uint64(ch.Code), userID, ch.Slug, status)
	return err
}
