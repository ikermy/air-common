package comdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ResponderBackfillStats reports the result of BackfillResponders.
type ResponderBackfillStats struct {
	Total     int
	Shared    int
	Orphans   int
	Copies    int
	Repointed int
	Merged    int
}

type responderRow struct {
	id     uint64
	typ    int
	name   sql.NullString
	realID sql.NullInt64
}

// BackfillResponders makes existing responders user-scoped.
//
// A responder used by several users keeps the smallest user id and gets a copy
// for every other user; dialogs are repointed to the copies. Unused responders
// are marked is_orphan and keep user_id = NULL. Duplicate identities per user
// are merged. The operation is idempotent.
func (d *DB) BackfillResponders(ctx context.Context) (ResponderBackfillStats, error) {
	var stats ResponderBackfillStats

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("backfill responders begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		"SELECT Id, Type, Name, RealId FROM responders ORDER BY Id")
	if err != nil {
		return stats, fmt.Errorf("backfill responders load: %w", err)
	}
	var responders []responderRow
	for rows.Next() {
		var r responderRow
		if err := rows.Scan(&r.id, &r.typ, &r.name, &r.realID); err != nil {
			rows.Close()
			return stats, fmt.Errorf("backfill responders scan: %w", err)
		}
		responders = append(responders, r)
	}
	if err := rows.Close(); err != nil {
		return stats, fmt.Errorf("backfill responders close: %w", err)
	}
	stats.Total = len(responders)

	usage := make(map[uint64][]uint32)
	usageRows, err := tx.QueryContext(ctx,
		"SELECT Responder, `User` FROM dialogs GROUP BY Responder, `User` ORDER BY Responder, `User`")
	if err != nil {
		return stats, fmt.Errorf("backfill responders usage: %w", err)
	}
	for usageRows.Next() {
		var (
			responderID uint64
			userID      uint32
		)
		if err := usageRows.Scan(&responderID, &userID); err != nil {
			usageRows.Close()
			return stats, fmt.Errorf("backfill responders usage scan: %w", err)
		}
		usage[responderID] = append(usage[responderID], userID)
	}
	if err := usageRows.Close(); err != nil {
		return stats, fmt.Errorf("backfill responders usage close: %w", err)
	}

	for _, r := range responders {
		users := usage[r.id]
		if len(users) == 0 {
			if _, err := tx.ExecContext(ctx,
				"UPDATE responders SET user_id = NULL, is_orphan = 1 WHERE Id = ?", r.id); err != nil {
				return stats, fmt.Errorf("backfill responders orphan %d: %w", r.id, err)
			}
			stats.Orphans++
			continue
		}

		if _, err := tx.ExecContext(ctx,
			"UPDATE responders SET user_id = ?, is_orphan = 0 WHERE Id = ?", users[0], r.id); err != nil {
			return stats, fmt.Errorf("backfill responders owner %d: %w", r.id, err)
		}
		if len(users) > 1 {
			stats.Shared++
		}

		for _, userID := range users[1:] {
			copyID, err := findResponder(ctx, tx, userID, r)
			if errors.Is(err, sql.ErrNoRows) {
				copyID = d.nextID()
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO responders (Id, user_id, Type, Name, RealId, is_orphan, Date)
					VALUES (?, ?, ?, ?, ?, 0, NOW())`,
					copyID, userID, r.typ, r.name, r.realID); err != nil {
					return stats, fmt.Errorf("backfill responders copy %d for user %d: %w", r.id, userID, err)
				}
				stats.Copies++
			} else if err != nil {
				return stats, fmt.Errorf("backfill responders find copy: %w", err)
			}

			res, err := tx.ExecContext(ctx,
				"UPDATE dialogs SET Responder = ? WHERE Responder = ? AND `User` = ?",
				copyID, r.id, userID)
			if err != nil {
				return stats, fmt.Errorf("backfill responders repoint %d -> %d: %w", r.id, copyID, err)
			}
			n, _ := res.RowsAffected()
			stats.Repointed += int(n)
		}
	}

	merged, err := mergeDuplicateResponders(ctx, tx)
	if err != nil {
		return stats, err
	}
	stats.Merged = merged

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("backfill responders commit: %w", err)
	}
	return stats, nil
}

// UserOwnsResponder reports whether responderID belongs to userID.
// Responders without an owner (is_orphan) never match.
func (d *DB) UserOwnsResponder(userID uint32, responderID uint64) (bool, error) {
	var exists int
	err := d.Conn().QueryRow(
		"SELECT 1 FROM responders WHERE Id = ? AND user_id = ? LIMIT 1",
		responderID, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("UserOwnsResponder: %w", err)
	}
	return true, nil
}

func findResponder(ctx context.Context, tx *sql.Tx, userID uint32, r responderRow) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, `
		SELECT Id FROM responders
		WHERE user_id = ? AND Type = ? AND RealId <=> ?
		ORDER BY Id LIMIT 1`, userID, r.typ, r.realID).Scan(&id)
	return id, err
}

func mergeDuplicateResponders(ctx context.Context, tx *sql.Tx) (int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT user_id, Type, RealId
		FROM responders
		WHERE user_id IS NOT NULL AND RealId IS NOT NULL
		GROUP BY user_id, Type, RealId
		HAVING COUNT(*) > 1`)
	if err != nil {
		return 0, fmt.Errorf("backfill responders duplicates: %w", err)
	}

	type group struct {
		userID uint32
		typ    int
		realID int64
	}
	var groups []group
	for rows.Next() {
		var g group
		if err := rows.Scan(&g.userID, &g.typ, &g.realID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("backfill responders duplicates scan: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("backfill responders duplicates close: %w", err)
	}

	merged := 0
	for _, g := range groups {
		ids, err := duplicateIDs(ctx, tx, g.userID, g.typ, g.realID)
		if err != nil {
			return merged, err
		}
		if len(ids) < 2 {
			continue
		}
		keeper := ids[0]
		for _, id := range ids[1:] {
			if _, err := tx.ExecContext(ctx,
				"UPDATE dialogs SET Responder = ? WHERE Responder = ?", keeper, id); err != nil {
				return merged, fmt.Errorf("backfill responders merge repoint %d -> %d: %w", id, keeper, err)
			}
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM responders WHERE Id = ?", id); err != nil {
				return merged, fmt.Errorf("backfill responders merge delete %d: %w", id, err)
			}
			merged++
		}
	}
	return merged, nil
}

func duplicateIDs(ctx context.Context, tx *sql.Tx, userID uint32, typ int, realID int64) ([]uint64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT Id FROM responders
		WHERE user_id = ? AND Type = ? AND RealId = ?
		ORDER BY Id`, userID, typ, realID)
	if err != nil {
		return nil, fmt.Errorf("backfill responders duplicate ids: %w", err)
	}
	defer rows.Close()

	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("backfill responders duplicate id scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
