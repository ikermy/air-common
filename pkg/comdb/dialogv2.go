package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/crypto"
)

// execer is implemented by both *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type legacyDialogMessage struct {
	Creator   flexInt         `json:"creator"`
	Message   json.RawMessage `json:"message"`
	Timestamp string          `json:"timestamp"`
}

// flexInt accepts both numeric and string JSON values.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*f = flexInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		*f = 0
		return nil
	}
	var parsed int
	if _, err := fmt.Sscanf(s, "%d", &parsed); err != nil {
		return err
	}
	*f = flexInt(parsed)
	return nil
}

// dialogRole maps the legacy creator value to the normalized message role.
func dialogRole(creator int) string {
	switch comdom.CreatorType(creator) {
	case comdom.AI, comdom.SpeechRealTimeAI:
		return "assistant"
	case comdom.User, comdom.UserVoice, comdom.SpeechRealTimeUser:
		return "user"
	case comdom.Operator:
		return "system"
	default:
		return "system"
	}
}

// insertDialogMessage writes one message into dialog_messages.
func (d *DB) insertDialogMessage(ctx context.Context, ex execer, dialogID, userID, seq uint64, raw json.RawMessage) error {
	normalized, err := normalizeLegacyMessageRaw(raw)
	if err != nil {
		return err
	}
	msg, err := parseLegacyMessage(normalized)
	if err != nil {
		return err
	}

	contentType := "text"
	var content sql.NullString
	contentJSON := sql.NullString{String: string(normalized), Valid: true}

	if len(msg.Message) > 0 {
		var text string
		if err := json.Unmarshal(msg.Message, &text); err == nil {
			content = sql.NullString{String: text, Valid: true}
		} else {
			contentType = "json"
			var obj struct {
				Message string `json:"message"`
				Audio   any    `json:"audio"`
				Image   any    `json:"image"`
			}
			if json.Unmarshal(msg.Message, &obj) == nil {
				if obj.Message != "" {
					content = sql.NullString{String: obj.Message, Valid: true}
					contentType = "text"
				}
				if obj.Audio != nil {
					contentType = "audio"
				}
				if obj.Image != nil {
					contentType = "image"
				}
			}
		}
	}

	createdAt := sql.NullTime{}
	if msg.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339Nano, msg.Timestamp); err == nil {
			createdAt = sql.NullTime{Time: ts, Valid: true}
		}
	}

	_, err = ex.ExecContext(ctx, `
		INSERT IGNORE INTO dialog_messages
			(id, dialog_id, user_id, seq, role, content_type, content, content_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE(?, NOW(3)))`,
		d.nextID(), dialogID, userID, seq, dialogRole(int(msg.Creator)), contentType,
		content, contentJSON, createdAt)
	return err
}

func normalizeLegacyMessageRaw(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, `"`) {
		var inner string
		if err := json.Unmarshal(raw, &inner); err != nil {
			return nil, fmt.Errorf("parse legacy dialog message: %w", err)
		}
		return json.RawMessage(inner), nil
	}
	return raw, nil
}

func parseLegacyMessage(raw json.RawMessage) (legacyDialogMessage, error) {
	var msg legacyDialogMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return msg, fmt.Errorf("parse legacy dialog message: %w", err)
	}
	return msg, nil
}

// readDialogLegacy reconstructs the legacy ReadDialog payload directly from
// dialogs.Data, without calling the stored function.
func (d *DB) readDialogLegacy(ctx context.Context, dialogID uint64, limit ...uint8) (json.RawMessage, error) {
	var (
		chatType      int
		userID        uint32
		date          sql.NullTime
		rawData       sql.NullString
		responderName sql.NullString
		userName      sql.NullString
	)
	err := d.Conn().QueryRowContext(ctx, `
		SELECT d.Type, d.User, d.Date, d.Data, r.Name,
			(SELECT g.Name FROM user_models um
			 LEFT JOIN user_gpt g ON um.ModelId = g.Id
			 WHERE um.UserId = d.User AND um.IsActive = 1
			 LIMIT 1)
		FROM dialogs d
		LEFT JOIN responders r ON r.Id = d.Responder
		WHERE d.Id = ?`, dialogID).
		Scan(&chatType, &userID, &date, &rawData, &responderName, &userName)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут при чтении диалога: %w", err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("диалог не найден")
		default:
			return nil, fmt.Errorf("ошибка чтения диалога: %w", err)
		}
	}

	var data any = []json.RawMessage{}
	if rawData.Valid && rawData.String != "" {
		payload := rawData.String
		if crypto.IsEncryptedWithMasterKey(payload) && d.MasterKeyResolver != nil {
			if mk, ok := d.MasterKeyResolver(userID); ok {
				if plain, err := crypto.DecryptFieldWithMasterKey(mk, payload); err == nil {
					payload = plain
				}
			}
		}

		var items []json.RawMessage
		normalized := d.normalizeDataArray(json.RawMessage(payload))
		if err := json.Unmarshal(normalized, &items); err == nil && items != nil {
			if len(limit) > 0 && limit[0] > 0 && len(items) > int(limit[0]) {
				items = items[len(items)-int(limit[0]):]
			}
			data = items
		} else {
			data = payload
		}
	}

	result := map[string]any{
		"Data":      data,
		"Type":      strconv.Itoa(chatType),
		"Model":     nullableString(responderName),
		"Responder": nullableString(userName),
	}
	if date.Valid {
		result["Date"] = date.Time.Format("2006-01-02 15:04:05")
	} else {
		result["Date"] = nil
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации диалога: %w", err)
	}
	return raw, nil
}

// readDialogV2 reconstructs the legacy ReadDialog payload from dialog_messages.
// It returns ok=false when the dialog is not migrated yet or v2 data is missing.
func (d *DB) readDialogV2(ctx context.Context, dialogID uint64, limit ...uint8) (json.RawMessage, bool) {
	var migrated bool
	if err := d.Conn().QueryRowContext(ctx,
		"SELECT messages_migrated FROM dialogs WHERE Id = ?", dialogID).Scan(&migrated); err != nil {
		return nil, false
	}
	if !migrated {
		d.backfillDialogMessages(ctx, dialogID)
		if err := d.Conn().QueryRowContext(ctx,
			"SELECT messages_migrated FROM dialogs WHERE Id = ?", dialogID).Scan(&migrated); err != nil || !migrated {
			return nil, false
		}
	}

	var (
		chatType      int
		date          sql.NullTime
		responderName sql.NullString
		userName      sql.NullString
	)
	if err := d.Conn().QueryRowContext(ctx, `
		SELECT d.Type, d.Date, r.Name,
			(SELECT g.Name FROM user_models um
			 LEFT JOIN user_gpt g ON um.ModelId = g.Id
			 WHERE um.UserId = d.User AND um.IsActive = 1
			 LIMIT 1)
		FROM dialogs d
		LEFT JOIN responders r ON r.Id = d.Responder
		WHERE d.Id = ?`, dialogID).
		Scan(&chatType, &date, &responderName, &userName); err != nil {
		return nil, false
	}

	query := "SELECT content_json, role, content, created_at FROM dialog_messages WHERE dialog_id = ? ORDER BY seq"
	args := []any{dialogID}
	if len(limit) > 0 && limit[0] > 0 {
		query = "SELECT content_json, role, content, created_at FROM dialog_messages WHERE dialog_id = ? ORDER BY seq DESC LIMIT ?"
		args = append(args, limit[0])
	}

	rows, err := d.Conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	items := make([]json.RawMessage, 0)
	for rows.Next() {
		var (
			contentJSON sql.NullString
			role        string
			content     sql.NullString
			createdAt   sql.NullTime
		)
		if err := rows.Scan(&contentJSON, &role, &content, &createdAt); err != nil {
			return nil, false
		}
		if contentJSON.Valid && json.Valid([]byte(contentJSON.String)) {
			items = append(items, json.RawMessage(contentJSON.String))
			continue
		}
		obj := map[string]any{
			"creator": legacyCreator(role),
			"message": content.String,
		}
		if createdAt.Valid {
			obj["timestamp"] = createdAt.Time.Format(time.RFC3339Nano)
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, false
		}
		items = append(items, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, false
	}

	if len(limit) > 0 && limit[0] > 0 {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}

	result := map[string]any{
		"Data":      items,
		"Type":      strconv.Itoa(chatType),
		"Model":     nullableString(responderName),
		"Responder": nullableString(userName),
	}
	if date.Valid {
		// Legacy ReadDialog returns the MySQL datetime representation.
		result["Date"] = date.Time.Format("2006-01-02 15:04:05")
	} else {
		result["Date"] = nil
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func legacyCreator(role string) int {
	switch role {
	case "assistant":
		return int(comdom.AI)
	case "user":
		return int(comdom.User)
	default:
		return int(comdom.Operator)
	}
}

func nullableString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}

func (d *DB) appendDialogMessageV2(ctx context.Context, ex execer, dialogID uint64, userID uint32, seq uint64, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	if err := d.insertDialogMessage(ctx, ex, dialogID, uint64(userID), seq, raw); err != nil {
		log.Printf("comdb: dialog_messages dual-write failed for dialog %d: %v", dialogID, err)
	}
}

// backfillDialogMessages migrates a legacy JSON array into dialog_messages.
func (d *DB) backfillDialogMessages(ctx context.Context, dialogID uint64) {
	var (
		userID   uint32
		migrated bool
		rawData  sql.NullString
	)
	if err := d.Conn().QueryRowContext(ctx,
		"SELECT `User`, messages_migrated, `Data` FROM dialogs WHERE Id = ?", dialogID).
		Scan(&userID, &migrated, &rawData); err != nil {
		return
	}
	if migrated || !rawData.Valid || rawData.String == "" {
		return
	}

	payload := rawData.String
	if d.MasterKeyResolver != nil && crypto.IsEncryptedWithMasterKey(payload) {
		if mk, ok := d.MasterKeyResolver(userID); ok {
			plain, err := crypto.DecryptFieldWithMasterKey(mk, payload)
			if err != nil {
				return
			}
			payload = plain
		} else {
			return
		}
	}

	normalized := d.normalizeDataArray(json.RawMessage(payload))
	var items []json.RawMessage
	if err := json.Unmarshal(normalized, &items); err != nil {
		return
	}

	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	for i, item := range items {
		if err := d.insertDialogMessage(ctx, tx, dialogID, uint64(userID), uint64(i), item); err != nil {
			log.Printf("comdb: dialog_messages backfill failed for dialog %d seq %d: %v", dialogID, i, err)
			return
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE dialogs SET messages_migrated = 1, messages_migrated_at = NOW(3) WHERE Id = ?", dialogID); err != nil {
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("comdb: dialog_messages backfill commit failed for dialog %d: %v", dialogID, err)
	}
}
