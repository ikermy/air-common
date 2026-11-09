package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// UserBotBackfillStats reports the result of BackfillUserBots.
type UserBotBackfillStats struct {
	Bots          int
	Events        int
	States        int
	SkippedEvents int
	SkippedStates int
}

type userBotRef struct {
	id     uint64
	userID uint64
}

type legacyBotRow struct {
	id        uint64
	userID    uint64
	botID     int64
	alias     string
	authData  string
	isActive  bool
	createdAt time.Time
	updatedAt time.Time
}

// BackfillUserBots copies service_tgbots/service_wabots into user_bots and
// rewrites service_user_bot_events/state with user-scoped user_bot_id.
// The operation is idempotent: bots are matched by (user_id, bot_type,
// external_bot_id), events keep legacy ids with INSERT IGNORE, state is upserted.
func (d *DB) BackfillUserBots(ctx context.Context) (UserBotBackfillStats, error) {
	var stats UserBotBackfillStats

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("backfill user_bots begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	refs := make(map[string]userBotRef)

	if err := d.backfillBotsTable(ctx, tx, "service_tgbots", "telegram", refs, &stats); err != nil {
		return stats, err
	}
	if err := d.backfillBotsTable(ctx, tx, "service_wabots", "whatsapp", refs, &stats); err != nil {
		return stats, err
	}
	if err := backfillUserBotEvents(ctx, tx, refs, &stats); err != nil {
		return stats, err
	}
	if err := backfillUserBotState(ctx, tx, refs, &stats); err != nil {
		return stats, err
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("backfill user_bots commit: %w", err)
	}
	return stats, nil
}

func (d *DB) backfillBotsTable(ctx context.Context, tx *sql.Tx, table, botType string, refs map[string]userBotRef, stats *UserBotBackfillStats) error {
	query := fmt.Sprintf(
		"SELECT Id, UserId, BotId, BotAlias, AuthData, IsActive, CreatedAt, UpdatedAt FROM `%s` ORDER BY Id", table)
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("backfill user_bots read %s: %w", table, err)
	}

	var source []legacyBotRow
	for rows.Next() {
		var (
			row   legacyBotRow
			botID sql.NullInt64
		)
		if err := rows.Scan(&row.id, &row.userID, &botID, &row.alias, &row.authData,
			&row.isActive, &row.createdAt, &row.updatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("backfill user_bots scan %s: %w", table, err)
		}
		if !botID.Valid {
			continue
		}
		row.botID = botID.Int64
		source = append(source, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("backfill user_bots iterate %s: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("backfill user_bots close %s: %w", table, err)
	}

	for _, row := range source {
		id, err := ensureUserBot(ctx, tx, d.nextID, row, botType)
		if err != nil {
			return err
		}
		refs[botKey(botType, row.botID)] = userBotRef{id: id, userID: row.userID}
		stats.Bots++
	}
	return nil
}

func ensureUserBot(ctx context.Context, tx *sql.Tx, nextID func() uint64, row legacyBotRow, botType string) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM user_bots
		WHERE user_id = ? AND bot_type = ? AND external_bot_id = ?`,
		row.userID, botType, row.botID).Scan(&id)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx, `
			UPDATE user_bots
			SET alias = ?, auth_ciphertext = ?, is_active = ?, created_at = ?, updated_at = ?
			WHERE id = ?`,
			row.alias, row.authData, row.isActive, row.createdAt, row.updatedAt, id); err != nil {
			return 0, fmt.Errorf("backfill user_bots update %d: %w", id, err)
		}
		return id, nil
	case errors.Is(err, sql.ErrNoRows):
		id = nextID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_bots (id, user_id, bot_type, external_bot_id, alias, auth_ciphertext, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, row.userID, botType, row.botID, row.alias, row.authData, row.isActive, row.createdAt, row.updatedAt); err != nil {
			return 0, fmt.Errorf("backfill user_bots insert %d/%d: %w", row.userID, row.botID, err)
		}
		return id, nil
	default:
		return 0, fmt.Errorf("backfill user_bots lookup %d/%d: %w", row.userID, row.botID, err)
	}
}

func backfillUserBotEvents(ctx context.Context, tx *sql.Tx, refs map[string]userBotRef, stats *UserBotBackfillStats) error {
	type eventRow struct {
		id                                 uint64
		botType                            string
		botID                              int64
		eventType                          string
		causeGroup, causeCode, description sql.NullString
		contextRaw                         sql.NullString
		severity                           string
		waitUntil                          sql.NullTime
		newIsActive                        sql.NullBool
		createdAt                          time.Time
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT Id, BotType, BotId, EventType, CauseGroup, CauseCode, Description,
		       Context, Severity, WaitUntil, NewIsActive, CreatedAt
		FROM service_user_bot_events
		ORDER BY Id`)
	if err != nil {
		return fmt.Errorf("backfill user_bot_events read: %w", err)
	}

	var events []eventRow
	for rows.Next() {
		var row eventRow
		if err := rows.Scan(&row.id, &row.botType, &row.botID, &row.eventType, &row.causeGroup,
			&row.causeCode, &row.description, &row.contextRaw, &row.severity, &row.waitUntil,
			&row.newIsActive, &row.createdAt); err != nil {
			rows.Close()
			return fmt.Errorf("backfill user_bot_events scan: %w", err)
		}
		events = append(events, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("backfill user_bot_events iterate: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("backfill user_bot_events close: %w", err)
	}

	for _, row := range events {
		ref, ok := refs[botKey(row.botType, row.botID)]
		if !ok {
			stats.SkippedEvents++
			continue
		}

		contextJSON := sql.NullString{}
		if row.contextRaw.Valid {
			value := row.contextRaw.String
			if !json.Valid([]byte(value)) {
				encoded, err := json.Marshal(value)
				if err != nil {
					return fmt.Errorf("backfill user_bot_events context: %w", err)
				}
				value = string(encoded)
			}
			contextJSON = sql.NullString{String: value, Valid: true}
		}

		res, err := tx.ExecContext(ctx, `
			INSERT IGNORE INTO user_bot_events
				(id, user_id, user_bot_id, event_type, cause_group, cause_code, description,
				 context_json, severity, wait_until, new_is_active, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.id, ref.userID, ref.id, row.eventType, row.causeGroup, row.causeCode, row.description,
			contextJSON, row.severity, row.waitUntil, row.newIsActive, row.createdAt)
		if err != nil {
			return fmt.Errorf("backfill user_bot_events insert %d: %w", row.id, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			stats.Events++
		}
	}

	return nil
}

func backfillUserBotState(ctx context.Context, tx *sql.Tx, refs map[string]userBotRef, stats *UserBotBackfillStats) error {
	type stateRow struct {
		botType      string
		botID        int64
		canStart     bool
		canSend      bool
		lastErrorGrp sql.NullString
		lastErrorCod sql.NullString
		lastErrorAt  sql.NullTime
		rateLimit    sql.NullTime
		updatedAt    time.Time
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT BotType, BotId, CanStart, CanSend, LastErrorGroup, LastErrorCode,
		       LastErrorAt, RateLimitUntil, UpdatedAt
		FROM service_user_bot_state`)
	if err != nil {
		return fmt.Errorf("backfill user_bot_state read: %w", err)
	}

	var states []stateRow
	for rows.Next() {
		var row stateRow
		if err := rows.Scan(&row.botType, &row.botID, &row.canStart, &row.canSend,
			&row.lastErrorGrp, &row.lastErrorCod, &row.lastErrorAt, &row.rateLimit, &row.updatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("backfill user_bot_state scan: %w", err)
		}
		states = append(states, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("backfill user_bot_state iterate: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("backfill user_bot_state close: %w", err)
	}

	for _, row := range states {
		ref, ok := refs[botKey(row.botType, row.botID)]
		if !ok {
			stats.SkippedStates++
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_bot_state
				(user_bot_id, user_id, can_start, can_send, last_error_group, last_error_code,
				 last_error_at, rate_limit_until, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				user_id = VALUES(user_id),
				can_start = VALUES(can_start),
				can_send = VALUES(can_send),
				last_error_group = VALUES(last_error_group),
				last_error_code = VALUES(last_error_code),
				last_error_at = VALUES(last_error_at),
				rate_limit_until = VALUES(rate_limit_until),
				updated_at = VALUES(updated_at)`,
			ref.id, ref.userID, row.canStart, row.canSend, row.lastErrorGrp, row.lastErrorCod,
			row.lastErrorAt, row.rateLimit, row.updatedAt); err != nil {
			return fmt.Errorf("backfill user_bot_state upsert %d: %w", ref.id, err)
		}
		stats.States++
	}
	return nil
}

func botKey(botType string, botID int64) string {
	return fmt.Sprintf("%s:%d", botType, botID)
}
