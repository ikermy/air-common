package comdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (d *DB) GetUserVectorStorage(userID uint32) (string, error) {
	// Проверяем входное значение
	if userID == 0 {
		return "", fmt.Errorf("получен некорректный userID")
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// SQL запрос для получения первого элемента VectorId из JSON активной модели
	// Используем новую структуру через user_models
	query := `
  SELECT JSON_UNQUOTE(JSON_EXTRACT(ug.Ids, '$.VectorId[0]'))
  FROM user_models um
  JOIN user_gpt ug ON um.ModelId = ug.Id
  WHERE um.userID = ? AND um.IsActive = 1
  LIMIT 1`

	var data sql.NullString
	err := d.Conn().QueryRowContext(ctx, query, userID).Scan(&data)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return "", fmt.Errorf("тайм-аут (%d с) при получении VectorStorage: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return "", fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return "", nil // Данные не найдены, но это не ошибка
		default:
			return "", fmt.Errorf("ошибка получения VectorStorage: %w", err)
		}
	}

	if !data.Valid {
		return "", nil // Возвращаем пустую строку если данные NULL
	}

	return data.String, nil
}

func (d *DB) GetOrSetUserStorageLimit(userID uint32, setStorage int64) (remaining uint64, totalLimit uint64, err error) {
	// Проверяем входное значение
	if userID == 0 {
		return 0, 0, fmt.Errorf("получен некорректный userID")
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	// Начинаем транзакцию
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback()

	// Получаем текущие значения с блокировкой строки
	var vLimit, vUsed int64
	err = tx.QueryRowContext(ctx, `
  SELECT quota_bytes, used_bytes
  FROM user_storage_quota
  WHERE user_id = ?
  FOR UPDATE`, userID).Scan(&vLimit, &vUsed)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return 0, 0, fmt.Errorf("тайм-аут (%d с) при получении лимитов хранилища: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return 0, 0, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return 0, 0, fmt.Errorf("подписка пользователя не найдена")
		default:
			return 0, 0, fmt.Errorf("ошибка получения лимитов хранилища: %w", err)
		}
	}

	// Вычисляем новое значение занятости
	vNewUsed := vUsed + setStorage

	// Гарантируем границы: [0, StorageLimit]
	if vNewUsed < 0 {
		vNewUsed = 0
	} else if vNewUsed > vLimit {
		vNewUsed = vLimit
	}

	// Обновляем значение StorageUsed
	_, err = tx.ExecContext(ctx, `
  UPDATE user_storage_quota
  SET used_bytes = ?
  WHERE user_id = ?`, vNewUsed, userID)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return 0, 0, fmt.Errorf("тайм-аут (%d с) при обновлении использования хранилища: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return 0, 0, fmt.Errorf("операция отменена: %w", err)
		default:
			return 0, 0, fmt.Errorf("ошибка обновления использования хранилища: %w", err)
		}
	}

	// Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}

	// Вычисляем оставшееся место и возвращаем результат
	remaining = uint64(vLimit - vNewUsed)
	totalLimit = uint64(vLimit)

	return remaining, totalLimit, nil
}
