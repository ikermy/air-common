package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/crypto"
	"github.com/ikermy/air-common/pkg/mode"
)

// ReadDialog читает всю историю диалога и возвращает структурированные данные
func (d *DB) ReadDialog(dialogId uint64, limit ...uint8) (json.RawMessage, error) {
	if dialogId == 0 {
		return nil, ErrInvalidDialogID
	}

	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// Lazy-миграция: заполняем dialog_messages из legacy Data при первом чтении.
	d.backfillDialogMessages(ctx, dialogId)

	if d.DialogV2ReadEnabled {
		if result, ok := d.readDialogV2(ctx, dialogId, limit...); ok {
			return result, nil
		}
	}

	return d.readDialogLegacy(ctx, dialogId, limit...)
}

// DeleteDialog удаляет диалог с проверкой прав пользователя.
// Каскад dialog_messages/dialog_contexts выполняет FK ON DELETE CASCADE.
func (d *DB) DeleteDialog(userID uint64, dialogId uint64) error {
	// Проверяем входные значения
	if dialogId == 0 {
		return ErrInvalidDialogID
	}
	if userID == 0 {
		return ErrInvalidUserID
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("deleteDialog select shard: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleteDialog begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var roleID int
	if err := tx.QueryRowContext(ctx,
		"SELECT RoleId FROM users WHERE Id = ?", userID).Scan(&roleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %d", ErrUserNotFound, userID)
		}
		return fmt.Errorf("deleteDialog role: %w", err)
	}
	if roleID == 1 {
		return ErrDemoUserDeletion
	}

	res, err := tx.ExecContext(ctx,
		"DELETE FROM dialogs WHERE Id = ? AND `User` = ?", dialogId, userID)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при удалении диалога: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка удаления диалога: %w", err)
		}
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrDialogNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("deleteDialog commit: %w", err)
	}
	return nil
}

// SaveDialog сохраняет всю историю диалога в базу данных.
// Читает + (при необходимости) расшифровывает + аппендит + шифрует + сохраняет.
// Использует транзакцию с FOR UPDATE для защиты от гонки.
func (d *DB) SaveDialog(treadId uint64, message json.RawMessage) error {
	if treadId == 0 {
		return fmt.Errorf("получен пустой тред")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	return d.saveDialog(ctx, treadId, message)
}

// saveDialogWithResolver читает + расшифровывает + аппендит + шифрует + сохраняет.
// Использует транзакцию с FOR UPDATE для защиты от гонки.
func (d *DB) saveDialog(ctx context.Context, treadId uint64, message json.RawMessage) error {
	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saveDialog begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Читаем userId и текущий Data с блокировкой строки
	var userId uint64
	var rawData sql.NullString
	if err = tx.QueryRowContext(ctx,
		"SELECT `User`, `Data` FROM dialogs WHERE Id = ? FOR UPDATE", treadId).
		Scan(&userId, &rawData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("диалог %d не найден", treadId)
		}
		return fmt.Errorf("saveDialog read: %w", err)
	}

	var (
		mk    [32]byte
		hasMK bool
	)
	if d.MasterKeyResolver != nil {
		mk, hasMK = d.MasterKeyResolver(userId)
	}

	// Разворачиваем текущий массив данных
	var arr []json.RawMessage
	if rawData.Valid && rawData.String != "" {
		data := rawData.String
		if hasMK && crypto.IsEncryptedWithMasterKey(data) {
			if data, err = crypto.DecryptFieldWithMasterKey(mk, data); err != nil {
				return fmt.Errorf("saveDialog decrypt: %w", err)
			}
		}
		// Нормализуем существующие данные перед аппендом
		rawArr := d.normalizeDataArray(json.RawMessage(data))
		_ = json.Unmarshal(rawArr, &arr)
	}

	// Аппендим новое сообщение
	seq := len(arr)
	arr = append(arr, message)

	newBytes, err := json.Marshal(arr)
	if err != nil {
		return fmt.Errorf("saveDialog marshal: %w", err)
	}
	newData := string(newBytes)

	// Шифруем если MasterKey доступен
	if hasMK {
		if newData, err = crypto.EncryptFieldWithMasterKey(mk, newData); err != nil {
			return fmt.Errorf("saveDialog encrypt: %w", err)
		}
	}

	if _, err = tx.ExecContext(ctx,
		"UPDATE dialogs SET `Data` = ?, `Date` = NOW() WHERE Id = ?",
		newData, treadId); err != nil {
		return fmt.Errorf("saveDialog update: %w", err)
	}

	d.appendDialogMessageV2(ctx, tx, treadId, userId, uint64(seq), message)

	return tx.Commit()
}

// UpdateDialogsMeta устанавливает достижение цели
func (d *DB) UpdateDialogsMeta(dialogId uint64, meta string) error {
	if dialogId == 0 {
		return fmt.Errorf("получен пустой dialogId")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	var query string
	switch meta {
	case "target":
		query = "UPDATE dialogs SET Target = 1 WHERE Id = ?"
	case "trigger":
		query = "UPDATE dialogs SET `Trigger` = 1 WHERE Id = ?"
	default:
		return fmt.Errorf("недопустимое значение meta: %q (ожидается target или trigger)", meta)
	}

	if _, err := d.Conn().ExecContext(ctx, query, dialogId); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при сохранении достижения цели: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка сохранения достижения цели: %w", err)
		}
	}

	return nil
}

// GetOrSetTreadAndResponder получает или создает тред и респондера.
// Реализация на Go: responder уникален в рамках (user_id, chat_type, real_id).
func (d *DB) GetOrSetTreadAndResponder(
	userID uint64,
	responderRealId uint64,
	responderName string,
	chatType comdom.ChannelType,
) (uint64, error) {
	if userID == 0 {
		return 0, fmt.Errorf("получен пустой userID")
	}
	if responderRealId > math.MaxInt64 {
		return 0, fmt.Errorf("responderRealId %d превышает BIGINT", responderRealId)
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("GetOrSetTreadAndResponder select shard: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("GetOrSetTreadAndResponder begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Сериализуем создание responder/dialog в рамках пользователя.
	var lockedUser uint32
	if err := tx.QueryRowContext(ctx,
		"SELECT Id FROM users WHERE Id = ? FOR UPDATE", userID).Scan(&lockedUser); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("пользователь %d не найден", userID)
		}
		return 0, fmt.Errorf("GetOrSetTreadAndResponder lock user: %w", err)
	}

	realID := int64(responderRealId)

	var responderID uint64
	err = tx.QueryRowContext(ctx, `
		SELECT Id FROM responders
		WHERE user_id = ? AND Type = ? AND RealId = ?
		ORDER BY Id LIMIT 1 FOR UPDATE`, userID, int(chatType), realID).Scan(&responderID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		responderID = d.nextID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO responders (Id, user_id, Type, Name, RealId, is_orphan, Date)
			VALUES (?, ?, ?, ?, ?, 0, NOW())`,
			responderID, userID, int(chatType), responderName, realID); err != nil {
			return 0, fmt.Errorf("GetOrSetTreadAndResponder insert responder: %w", err)
		}
	case err != nil:
		return 0, fmt.Errorf("GetOrSetTreadAndResponder find responder: %w", err)
	default:
		if responderName != "" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE responders SET Name = ?
				WHERE Id = ? AND (Name IS NULL OR Name <> ?)`,
				responderName, responderID, responderName); err != nil {
				return 0, fmt.Errorf("GetOrSetTreadAndResponder update responder: %w", err)
			}
		}
	}

	var dialogID uint64
	err = tx.QueryRowContext(ctx,
		"SELECT Id FROM dialogs WHERE `User` = ? AND Responder = ? AND Type = ? ORDER BY Id LIMIT 1 FOR UPDATE",
		userID, responderID, int(chatType)).Scan(&dialogID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		dialogID = d.nextID()
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO dialogs (Id, id_source, `User`, Responder, Type, Date) VALUES (?, 'snowflake', ?, ?, ?, NOW())",
			dialogID, userID, responderID, int(chatType)); err != nil {
			return 0, fmt.Errorf("GetOrSetTreadAndResponder insert dialog: %w", err)
		}
	case err != nil:
		return 0, fmt.Errorf("GetOrSetTreadAndResponder find dialog: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("GetOrSetTreadAndResponder commit: %w", err)
	}
	return dialogID, nil
}
