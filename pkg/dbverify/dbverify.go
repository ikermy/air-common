// Package dbverify compares user-scoped rows between two databases.
//
// It is intended for shard cutover: before switching a slot, verify that the
// target shard contains the same row counts and primary-key checksums as the
// source for every user in the slot.
package dbverify

import (
	"context"
	"database/sql"
	"fmt"
)

// Table describes a user-scoped table.
type Table struct {
	Name string
	// UserColumn is the column that references the user.
	UserColumn string
	// KeyColumn is the primary key (or a unique key) used for the checksum.
	KeyColumn string
}

// Diff is a mismatch for one user and table.
type Diff struct {
	UserID uint64
	Table  string
	Source Checksum
	Target Checksum
	Reason string
}

// Checksum is a row count plus a key-based checksum.
type Checksum struct {
	Count uint64
	Sum   int64
}

// DefaultTables lists user-scoped tables in the current schema.
func DefaultTables() []Table {
	return []Table{
		{Name: "users", UserColumn: "Id", KeyColumn: "Id"},
		{Name: "user_auth", UserColumn: "UserId", KeyColumn: "Id"},
		{Name: "user_models", UserColumn: "UserId", KeyColumn: "Id"},
		{Name: "user_api_keys", UserColumn: "UserId", KeyColumn: "Id"},
		{Name: "user_channels", UserColumn: "user_id", KeyColumn: "id"},
		{Name: "user_channel_configs", UserColumn: "user_id", KeyColumn: "channel_id"},
		{Name: "user_bots", UserColumn: "user_id", KeyColumn: "id"},
		{Name: "user_bot_events", UserColumn: "user_id", KeyColumn: "id"},
		{Name: "user_bot_state", UserColumn: "user_id", KeyColumn: "user_bot_id"},
		{Name: "user_storage_config", UserColumn: "user_id", KeyColumn: "user_id"},
		{Name: "user_storage_quota", UserColumn: "user_id", KeyColumn: "user_id"},
		{Name: "dialogs", UserColumn: "User", KeyColumn: "Id"},
		{Name: "responders", UserColumn: "user_id", KeyColumn: "Id"},
		{Name: "subscriptions", UserColumn: "UserId", KeyColumn: "Id"},
		{Name: "notifications", UserColumn: "UserId", KeyColumn: "UserId"},
		{Name: "billing", UserColumn: "UserId", KeyColumn: "Id"},
	}
}

// CompareUser compares one user across all tables.
func CompareUser(ctx context.Context, source, target *sql.DB, userID uint64, tables []Table) ([]Diff, error) {
	if source == nil || target == nil {
		return nil, fmt.Errorf("dbverify: source and target are required")
	}
	if len(tables) == 0 {
		tables = DefaultTables()
	}

	var diffs []Diff
	for _, table := range tables {
		src, err := checksum(ctx, source, table, userID)
		if err != nil {
			return nil, fmt.Errorf("dbverify: source %s: %w", table.Name, err)
		}
		dst, err := checksum(ctx, target, table, userID)
		if err != nil {
			return nil, fmt.Errorf("dbverify: target %s: %w", table.Name, err)
		}
		if src == dst {
			continue
		}
		diffs = append(diffs, Diff{
			UserID: userID,
			Table:  table.Name,
			Source: src,
			Target: dst,
			Reason: "checksum mismatch",
		})
	}
	return diffs, nil
}

// CompareUsers compares several users and aggregates all diffs.
func CompareUsers(ctx context.Context, source, target *sql.DB, userIDs []uint64, tables []Table) ([]Diff, error) {
	var all []Diff
	for _, userID := range userIDs {
		diffs, err := CompareUser(ctx, source, target, userID, tables)
		if err != nil {
			return all, err
		}
		all = append(all, diffs...)
	}
	return all, nil
}

func checksum(ctx context.Context, db *sql.DB, table Table, userID uint64) (Checksum, error) {
	query := fmt.Sprintf(
		"SELECT COUNT(*), COALESCE(SUM(CRC32(`%s`)), 0) FROM `%s` WHERE `%s` = ?",
		table.KeyColumn, table.Name, table.UserColumn)

	var c Checksum
	if err := db.QueryRowContext(ctx, query, userID).Scan(&c.Count, &c.Sum); err != nil {
		return Checksum{}, err
	}
	return c, nil
}
