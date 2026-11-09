// Package outbox implements the transactional outbox pattern on top of the
// `outbox` table (migration 000015).
//
// Enqueue must be called with the same *sql.Tx as the domain change. A worker
// claims rows with FOR UPDATE SKIP LOCKED, invokes a handler and marks the row
// done or failed (with backoff). Delivery is at-least-once.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Status values stored in the outbox table.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusDone       = "done"
	StatusFailed     = "failed"
)

// Event is one outbox row.
type Event struct {
	ID            uint64
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       json.RawMessage
	Attempts      int
}

// Validate checks required fields.
func (e Event) Validate() error {
	if e.AggregateType == "" {
		return fmt.Errorf("outbox: aggregate_type is required")
	}
	if e.AggregateID == "" {
		return fmt.Errorf("outbox: aggregate_id is required")
	}
	if e.EventType == "" {
		return fmt.Errorf("outbox: event_type is required")
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return fmt.Errorf("outbox: payload must be valid JSON")
	}
	return nil
}

// Store provides outbox persistence.
type Store struct {
	DB *sql.DB
}

// New returns a Store.
func New(db *sql.DB) *Store { return &Store{DB: db} }

// Enqueue inserts an event inside the caller's transaction.
func (s *Store) Enqueue(ctx context.Context, tx *sql.Tx, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if tx == nil {
		return fmt.Errorf("outbox: tx is required")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE payload = VALUES(payload)`,
		e.AggregateType, e.AggregateID, e.EventType, []byte(e.Payload))
	if err != nil {
		return fmt.Errorf("outbox enqueue: %w", err)
	}
	return nil
}

// Claim locks and returns up to limit ready events. Failed events are retried
// when available_at is reached.
func (s *Store) Claim(ctx context.Context, limit int, lockFor time.Duration) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if lockFor <= 0 {
		lockFor = time.Minute
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("outbox claim: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, attempts
		FROM outbox
		WHERE status IN ('pending','failed')
		  AND available_at <= NOW(3)
		  AND (locked_until IS NULL OR locked_until < NOW(3))
		ORDER BY id
		LIMIT ?
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox claim: select: %w", err)
	}

	var events []Event
	for rows.Next() {
		var e Event
		var payload []byte
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType, &payload, &e.Attempts); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("outbox claim: scan: %w", err)
		}
		e.Payload = payload
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("outbox claim: rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("outbox claim: close: %w", err)
	}

	for _, e := range events {
		if _, err := tx.ExecContext(ctx, `
			UPDATE outbox SET status = 'processing', locked_until = DATE_ADD(NOW(3), INTERVAL ? MICROSECOND)
			WHERE id = ?`, lockFor.Microseconds(), e.ID); err != nil {
			return nil, fmt.Errorf("outbox claim: lock %d: %w", e.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("outbox claim: commit: %w", err)
	}
	return events, nil
}

// MarkDone marks an event as delivered.
func (s *Store) MarkDone(ctx context.Context, id uint64) error {
	_, err := s.DB.ExecContext(ctx,
		"UPDATE outbox SET status = 'done', locked_until = NULL, last_error = NULL WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("outbox mark done %d: %w", id, err)
	}
	return nil
}

// MarkFailed marks an event as failed and schedules a retry after backoff.
func (s *Store) MarkFailed(ctx context.Context, id uint64, cause error, backoff time.Duration) error {
	if backoff <= 0 {
		backoff = time.Minute
	}
	errText := ""
	if cause != nil {
		errText = cause.Error()
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE outbox
		SET status = 'failed',
		    attempts = attempts + 1,
		    locked_until = NULL,
		    last_error = ?,
		    available_at = DATE_ADD(NOW(3), INTERVAL ? MICROSECOND)
		WHERE id = ?`, errText, backoff.Microseconds(), id)
	if err != nil {
		return fmt.Errorf("outbox mark failed %d: %w", id, err)
	}
	return nil
}

// RequeueStale returns processing rows with an expired lock to the pending state.
func (s *Store) RequeueStale(ctx context.Context) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE outbox SET status = 'pending', locked_until = NULL
		WHERE status = 'processing' AND locked_until IS NOT NULL AND locked_until < NOW(3)`)
	if err != nil {
		return 0, fmt.Errorf("outbox requeue stale: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("outbox requeue stale rows: %w", err)
	}
	return n, nil
}

// Handler processes one event.
type Handler func(context.Context, Event) error

// Dispatch claims and processes one batch. Handler errors are retried with the
// supplied backoff; the function returns the number of successfully handled
// events and the first infrastructure error.
func (s *Store) Dispatch(ctx context.Context, limit int, lockFor, backoff time.Duration, h Handler) (int, error) {
	if h == nil {
		return 0, fmt.Errorf("outbox: handler is required")
	}
	events, err := s.Claim(ctx, limit, lockFor)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, e := range events {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		if err := h(ctx, e); err != nil {
			if failErr := s.MarkFailed(ctx, e.ID, err, backoff); failErr != nil {
				return processed, failErr
			}
			continue
		}
		if err := s.MarkDone(ctx, e.ID); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
