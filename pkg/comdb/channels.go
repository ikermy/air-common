package comdb

import (
	"context"
	"fmt"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/mode"
)

// SaveChannelDataRaw stores a channel payload in the legacy channels table and
// in user_channels/user_channel_configs. The payload is stored as-is: callers
// are responsible for encryption.
func (d *DB) SaveChannelDataRaw(userID uint32, channelType, payload string, enabled bool) error {
	if userID == 0 {
		return fmt.Errorf("получен пустой userID")
	}
	ch, ok := comdom.ChannelBySlug(channelType)
	if !ok || !ch.Configurable || ch.LegacyColumn == "" {
		return fmt.Errorf("неизвестный тип канала: %s", channelType)
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("saveChannelData select shard: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saveChannelData begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM channels WHERE UserId = ?)", userID).Scan(&exists); err != nil {
		return fmt.Errorf("ошибка проверки каналов: %w", err)
	}
	if !exists {
		if _, err := tx.ExecContext(ctx, "INSERT INTO channels (UserId) VALUES (?)", userID); err != nil {
			return fmt.Errorf("ошибка создания записи channels: %w", err)
		}
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	query := fmt.Sprintf("UPDATE channels SET `%s` = ?, `%s_enabled` = ? WHERE UserId = ?", ch.LegacyColumn, ch.LegacyColumn)
	if _, err := tx.ExecContext(ctx, query, payload, enabledInt, userID); err != nil {
		return fmt.Errorf("ошибка обновления канала %s: %w", channelType, err)
	}

	channelID := uint64(userID)*10 + uint64(ch.Code)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_channels (id, user_id, channel_type, enabled)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE enabled = VALUES(enabled)`,
		channelID, userID, ch.Slug, enabledInt); err != nil {
		return fmt.Errorf("ошибка записи user_channels: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		"SELECT id FROM user_channels WHERE user_id = ? AND channel_type = ?",
		userID, ch.Slug).Scan(&channelID); err != nil {
		return fmt.Errorf("ошибка чтения user_channels: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_channel_configs (channel_id, user_id, config)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE config = VALUES(config)`,
		channelID, userID, payload); err != nil {
		return fmt.Errorf("ошибка записи user_channel_configs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("saveChannelData commit: %w", err)
	}
	return nil
}
