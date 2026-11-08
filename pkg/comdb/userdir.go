package comdb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ikermy/air-common/pkg/dbrouter"
)

// UserDirectoryBackfillStats reports the result of BackfillUserDirectory.
type UserDirectoryBackfillStats struct {
	Total    int
	Inserted int
	Existing int
	Skipped  int
}

// BackfillUserDirectory fills user_directory from user_auth.
//
// slot = dbrouter.SlotForUser(user_id). shard_id is derived with shardForSlot
// (when nil, shard 0 is used). Invalid hex email hashes and rows without an
// email hash are skipped. The operation is idempotent.
func (d *DB) BackfillUserDirectory(ctx context.Context, shardForSlot func(slot uint16) uint16) (UserDirectoryBackfillStats, error) {
	var stats UserDirectoryBackfillStats

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	rows, err := d.Conn().QueryContext(ctx, `
		SELECT ua.UserId, ua.EmailHash
		FROM user_auth ua
		LEFT JOIN user_directory ud ON ud.user_id = ua.UserId
		WHERE ud.user_id IS NULL
		  AND ua.EmailHash IS NOT NULL
		  AND ua.EmailHash <> ''`)
	if err != nil {
		return stats, fmt.Errorf("backfill user_directory load: %w", err)
	}

	type entry struct {
		userID    uint64
		emailHash []byte
	}
	var entries []entry
	for rows.Next() {
		var (
			userID uint64
			hash   sql.NullString
		)
		if err := rows.Scan(&userID, &hash); err != nil {
			rows.Close()
			return stats, fmt.Errorf("backfill user_directory scan: %w", err)
		}
		stats.Total++
		if !hash.Valid || hash.String == "" {
			stats.Skipped++
			continue
		}
		decoded, err := hex.DecodeString(hash.String)
		if err != nil || len(decoded) != 32 {
			stats.Skipped++
			continue
		}
		entries = append(entries, entry{userID: userID, emailHash: decoded})
	}
	if err := rows.Close(); err != nil {
		return stats, fmt.Errorf("backfill user_directory close: %w", err)
	}

	for _, e := range entries {
		slot := dbrouter.SlotForUser(e.userID)
		shard := uint16(0)
		if shardForSlot != nil {
			shard = shardForSlot(slot)
		}
		res, err := d.Conn().ExecContext(ctx, `
			INSERT INTO user_directory (user_id, email_hash, slot, shard_id)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE slot = VALUES(slot), shard_id = VALUES(shard_id)`,
			e.userID, e.emailHash, slot, shard)
		if err != nil {
			return stats, fmt.Errorf("backfill user_directory insert user %d: %w", e.userID, err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			stats.Existing++
		} else {
			stats.Inserted++
		}
	}

	return stats, nil
}

// RecomputeUserDirectoryShards updates shard_id for every directory row whose
// current value differs from shardForSlot(slot). It is used after a slot-map
// generation change. Returns the number of updated rows.
func (d *DB) RecomputeUserDirectoryShards(ctx context.Context, shardForSlot func(slot uint16) uint16) (int, error) {
	if shardForSlot == nil {
		return 0, fmt.Errorf("RecomputeUserDirectoryShards: resolver is required")
	}

	const batchSize = 1000
	var (
		lastID  uint64
		updated int
	)

	for {
		rows, err := d.Conn().QueryContext(ctx, `
			SELECT user_id, slot, shard_id
			FROM user_directory
			WHERE user_id > ?
			ORDER BY user_id
			LIMIT ?`, lastID, batchSize)
		if err != nil {
			return updated, fmt.Errorf("RecomputeUserDirectoryShards: load: %w", err)
		}

		type record struct {
			userID uint64
			slot   uint16
			shard  uint16
		}
		var batch []record
		for rows.Next() {
			var r record
			if err := rows.Scan(&r.userID, &r.slot, &r.shard); err != nil {
				rows.Close()
				return updated, fmt.Errorf("RecomputeUserDirectoryShards: scan: %w", err)
			}
			batch = append(batch, r)
		}
		if err := rows.Close(); err != nil {
			return updated, fmt.Errorf("RecomputeUserDirectoryShards: close: %w", err)
		}
		if len(batch) == 0 {
			break
		}
		lastID = batch[len(batch)-1].userID

		for _, r := range batch {
			want := shardForSlot(r.slot)
			if r.shard == want {
				continue
			}
			if _, err := d.Conn().ExecContext(ctx,
				"UPDATE user_directory SET shard_id = ? WHERE user_id = ?", want, r.userID); err != nil {
				return updated, fmt.Errorf("RecomputeUserDirectoryShards: update user %d: %w", r.userID, err)
			}
			updated++
		}
	}

	return updated, nil
}

// UserDirectoryIssue describes a consistency problem in user_directory.
type UserDirectoryIssue struct {
	UserID uint64
	Reason string
}

// VerifyUserDirectory checks that user_directory is consistent with user_auth
// and the slot function. It does not modify data.
func (d *DB) VerifyUserDirectory(ctx context.Context, shardForSlot func(slot uint16) uint16) ([]UserDirectoryIssue, error) {
	var issues []UserDirectoryIssue

	rows, err := d.Conn().QueryContext(ctx, `
		SELECT ua.UserId
		FROM user_auth ua
		LEFT JOIN user_directory ud ON ud.user_id = ua.UserId
		WHERE ud.user_id IS NULL
		  AND ua.EmailHash IS NOT NULL
		  AND ua.EmailHash <> ''`)
	if err != nil {
		return nil, fmt.Errorf("VerifyUserDirectory: missing rows: %w", err)
	}
	for rows.Next() {
		var userID uint64
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("VerifyUserDirectory: missing scan: %w", err)
		}
		issues = append(issues, UserDirectoryIssue{UserID: userID, Reason: "missing directory row"})
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("VerifyUserDirectory: missing close: %w", err)
	}

	rows, err = d.Conn().QueryContext(ctx, `
		SELECT ud.user_id
		FROM user_directory ud
		LEFT JOIN user_auth ua ON ua.UserId = ud.user_id
		WHERE ua.UserId IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("VerifyUserDirectory: orphan rows: %w", err)
	}
	for rows.Next() {
		var userID uint64
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("VerifyUserDirectory: orphan scan: %w", err)
		}
		issues = append(issues, UserDirectoryIssue{UserID: userID, Reason: "directory row without user_auth"})
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("VerifyUserDirectory: orphan close: %w", err)
	}

	rows, err = d.Conn().QueryContext(ctx, "SELECT user_id, slot, shard_id FROM user_directory")
	if err != nil {
		return nil, fmt.Errorf("VerifyUserDirectory: routing rows: %w", err)
	}
	for rows.Next() {
		var (
			userID uint64
			slot   uint16
			shard  uint16
		)
		if err := rows.Scan(&userID, &slot, &shard); err != nil {
			rows.Close()
			return nil, fmt.Errorf("VerifyUserDirectory: routing scan: %w", err)
		}
		if want := dbrouter.SlotForUser(userID); slot != want {
			issues = append(issues, UserDirectoryIssue{
				UserID: userID,
				Reason: fmt.Sprintf("slot mismatch: got %d, want %d", slot, want),
			})
			continue
		}
		if shardForSlot != nil {
			if want := shardForSlot(slot); shard != want {
				issues = append(issues, UserDirectoryIssue{
					UserID: userID,
					Reason: fmt.Sprintf("shard mismatch: got %d, want %d", shard, want),
				})
			}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("VerifyUserDirectory: routing close: %w", err)
	}

	return issues, nil
}

// UpsertUserDirectory records the shard location for a user.
func (d *DB) UpsertUserDirectory(ctx context.Context, userID uint64, emailHash []byte, slot, shard uint16) error {
	if len(emailHash) != 32 {
		return fmt.Errorf("UpsertUserDirectory: email hash must be 32 bytes, got %d", len(emailHash))
	}
	_, err := d.Conn().ExecContext(ctx, `
		INSERT INTO user_directory (user_id, email_hash, slot, shard_id)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE slot = VALUES(slot), shard_id = VALUES(shard_id)`,
		userID, emailHash, slot, shard)
	if err != nil {
		return fmt.Errorf("UpsertUserDirectory: %w", err)
	}
	return nil
}

// LookupUserDirectoryByEmailHash resolves an email hash to its user and shard.
func (d *DB) LookupUserDirectoryByEmailHash(ctx context.Context, emailHash []byte) (userID uint64, slot, shard uint16, err error) {
	if len(emailHash) != 32 {
		return 0, 0, 0, fmt.Errorf("LookupUserDirectoryByEmailHash: email hash must be 32 bytes, got %d", len(emailHash))
	}
	err = d.Conn().QueryRowContext(ctx,
		"SELECT user_id, slot, shard_id FROM user_directory WHERE email_hash = ?",
		emailHash).Scan(&userID, &slot, &shard)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, sql.ErrNoRows
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("LookupUserDirectoryByEmailHash: %w", err)
	}
	return userID, slot, shard, nil
}

// LookupUserDirectory resolves a user id to its slot and shard.
func (d *DB) LookupUserDirectory(ctx context.Context, userID uint64) (slot, shard uint16, err error) {
	err = d.Conn().QueryRowContext(ctx,
		"SELECT slot, shard_id FROM user_directory WHERE user_id = ?",
		userID).Scan(&slot, &shard)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, sql.ErrNoRows
	}
	if err != nil {
		return 0, 0, fmt.Errorf("LookupUserDirectory: %w", err)
	}
	return slot, shard, nil
}

// DecodeEmailHash decodes a hex-encoded SHA-256 email hash.
func DecodeEmailHash(encoded string) ([]byte, error) {
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("DecodeEmailHash: %w", err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("DecodeEmailHash: expected 32 bytes, got %d", len(decoded))
	}
	return decoded, nil
}
