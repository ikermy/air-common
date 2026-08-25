package comdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ikermy/air-common/pkg/crypto"
	"github.com/ikermy/air-common/pkg/mode"
)

// ReadDialog читает всю историю диалога и возвращает структурированные данные
func (d *DB) ReadDialog(dialogId uint64, limit ...uint8) (json.RawMessage, error) {
	if dialogId == 0 {
		return nil, fmt.Errorf("получен некорректный dialogId")
	}

	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	var raw sql.NullString
	var err error
	if len(limit) == 0 {
		err = d.Conn().QueryRowContext(ctx, "SELECT ReadDialog(?, NULL);", dialogId).Scan(&raw)
	} else {
		err = d.Conn().QueryRowContext(ctx, "SELECT ReadDialog(?, ?);", dialogId, limit[0]).Scan(&raw)
	}
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут при вызове ReadDialog: %w", err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("диалог не найден")
		default:
			return nil, fmt.Errorf("ошибка ReadDialog: %w", err)
		}
	}
	if !raw.Valid {
		return nil, fmt.Errorf("получены пустые данные")
	}

	result := json.RawMessage(raw.String)

	// Обрабатываем результат: расшифровка (если нужно) и нормализация массива Data
	result = d.processReadDialogResult(ctx, dialogId, result)

	return result, nil
}

// DeleteDialog удаляет диалог с проверкой прав пользователя
func (d *DB) DeleteDialog(userID uint32, dialogId uint64) error {
	// Проверяем входные значения
	if dialogId == 0 {
		return fmt.Errorf("получен некорректный dialogId")
	}
	if userID == 0 {
		return fmt.Errorf("получен некорректный userID")
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// Вызываем хранимую процедуру с проверкой прав
	_, err := d.Conn().ExecContext(ctx, "CALL DeleteDialog(?, ?)", dialogId, userID)
	if err != nil {
		// Проверяем специальный код ошибки для демо-пользователя
		if strings.Contains(err.Error(), "SQLSTATE 45001") ||
			strings.Contains(err.Error(), "Невозможно удалить диалог демо пользователя") {
			return fmt.Errorf("демо пользователь не может удалять диалоги")
		}

		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при удалении диалога: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка удаления диалога: %w", err)
		}
	}

	return nil
}

// SaveDialog сохраняет всю историю диалога в базу данных
func (d *DB) SaveDialog(treadId uint64, message json.RawMessage) error {
	if treadId == 0 {
		return fmt.Errorf("получен пустой тред")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	// Если resolver задан — обрабатываем шифрование сами (минуя SP)
	if d.MasterKeyResolver != nil {
		return d.saveDialogWithResolver(ctx, treadId, message)
	}

	// Fallback: хранимая процедура (plaintext, обратная совместимость)
	if _, err := d.Conn().ExecContext(ctx, "CALL SaveDialog(?, ?)", treadId, message); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут при сохранении диалога: %w", err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка сохранения диалога: %w", err)
		}
	}
	return nil
}

// saveDialogWithResolver читает + расшифровывает + аппендит + шифрует + сохраняет.
// Использует транзакцию с FOR UPDATE для защиты от гонки.
func (d *DB) saveDialogWithResolver(ctx context.Context, treadId uint64, message json.RawMessage) error {
	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saveDialog begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Читаем userId и текущий Data с блокировкой строки
	var userId uint32
	var rawData sql.NullString
	if err = tx.QueryRowContext(ctx,
		"SELECT `User`, `Data` FROM dialogs WHERE Id = ? FOR UPDATE", treadId).
		Scan(&userId, &rawData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("диалог %d не найден", treadId)
		}
		return fmt.Errorf("saveDialog read: %w", err)
	}

	mk, hasMK := d.MasterKeyResolver(userId)

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

	return tx.Commit()
}

// UpdateDialogsMeta устанавливает достижение цели
func (d *DB) UpdateDialogsMeta(dialogId uint64, meta string) error {
	if dialogId == 0 {
		return fmt.Errorf("получен пустой dialogId")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	if _, err := d.Conn().ExecContext(ctx, "CALL UpdateDialogsMeta(?,?)", dialogId, meta); err != nil {
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

// GetOrSetTreadAndResponder получает или создает тред и респондера
func (d *DB) GetOrSetTreadAndResponder(
	userID uint32,
	responderRealId uint64,
	responderName string,
	chatType ChatType,
) (uint64, error) {
	if userID == 0 {
		return 0, fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	// Создаём временную переменную для выхода
	if _, err := d.Conn().ExecContext(ctx, "SET @out_dialogId = 0;"); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return 0, fmt.Errorf("тайм-аут (%d с) при создании временной переменной: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return 0, fmt.Errorf("операция отменена: %w", err)
		default:
			return 0, fmt.Errorf("ошибка при создании временной переменной: %w", err)
		}
	}

	// Выполняем вызов процедуры
	if _, err := d.Conn().ExecContext(ctx, "CALL GetOrSetTreadAndResponder(?, ?, ?, ?, @out_dialogId);",
		userID, responderRealId, responderName, chatType); err != nil { // Тип чата TgBot
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return 0, fmt.Errorf("тайм-аут (%d с) при вызове процедуры GetOrSetTreadAndResponder: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return 0, fmt.Errorf("операция отменена: %w", err)
		default:
			return 0, fmt.Errorf("ошибка вызова процедуры GetOrSetTreadAndResponder: %w", err)
		}
	}

	// Читаем значение из переменной
	var dialogId uint64
	if err := d.Conn().QueryRowContext(ctx, "SELECT @out_dialogId;").Scan(&dialogId); err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return 0, fmt.Errorf("тайм-аут (%d с) при получении значения @out_dialogId: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return 0, fmt.Errorf("операция отменена: %w", err)
		default:
			return 0, fmt.Errorf("ошибка получения значения @out_dialogId: %w", err)
		}
	}

	return dialogId, nil
}
