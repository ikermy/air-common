package comdb

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/mode"
)

// GetAllUserModels получает все модели пользователя из таблицы user_models
func (d *DB) GetAllUserModels(userID uint32) ([]comdom.UserModelRecord, error) {
	if userID == 0 {
		return nil, fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	rows, err := d.Conn().QueryContext(ctx,
		`SELECT 
			um.ModelId AS UserModelId,
			um.Provider AS ProviderName,
			um.IsActive AS IsActive,
			ug.AssistantId AS UserModelAssistId,
			gm.Id AS GptModelId,
			gm.Name AS GptModelName,
			rm.Id AS RealtimeModelId,
			rm.Name AS RealtimeModelName,
			ug.Ids AS Ids
		FROM user_models um
		JOIN user_gpt ug ON um.ModelId = ug.Id
		LEFT JOIN gpt_models gm ON gm.Id = ug.Model
		LEFT JOIN realtime_models rm ON rm.Id = ug.Realtime
		WHERE um.UserId = ?
		ORDER BY um.IsActive DESC, um.CreatedAt DESC`, userID)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при получении моделей: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		default:
			return nil, fmt.Errorf("ошибка получения моделей: %w", err)
		}
	}
	defer rows.Close()

	var records []comdom.UserModelRecord
	for rows.Next() {
		var record comdom.UserModelRecord
		var isActive int8
		var idsRaw sql.NullString
		var gptModelID sql.NullInt64
		var gptModelName sql.NullString
		var realtimeModelID sql.NullInt64
		var realtimeModelName sql.NullString

		if err := rows.Scan(&record.ModelId, &record.Provider, &isActive, &record.AssistId,
			&gptModelID, &gptModelName, &realtimeModelID, &realtimeModelName, &idsRaw); err != nil {
			return nil, fmt.Errorf("ошибка чтения модели пользователя: %w", err)
		}

		record.IsActive = isActive == 1
		if gptModelName.Valid && gptModelID.Valid {
			record.GptType = &comdom.GptType{
				ID:   uint(gptModelID.Int64),
				Name: gptModelName.String,
			}
		}
		if realtimeModelName.Valid && realtimeModelID.Valid {
			record.Realtime = &comdom.Realtime{
				ID:   uint(realtimeModelID.Int64),
				Name: realtimeModelName.String,
			}
		}

		// Парсим JSON из поля Ids
		if idsRaw.Valid && idsRaw.String != "" {
			// Сохраняем raw JSON в AllIds для доступа к VectorId
			record.AllIds = []byte(idsRaw.String)

			// Парсим FileIds для обратной совместимости
			var data struct {
				FileIds  []comdom.Ids `json:"FileIds"`
				VectorId []string     `json:"VectorId"`
			}
			if err := json.Unmarshal([]byte(idsRaw.String), &data); err != nil {
			} else {
				record.FileIds = data.FileIds
			}
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка при итерации по записям: %w", err)
	}

	return records, nil
}

// RemoveModelFromUser удаляет связь между пользователем и моделью в таблице user_models
// Также удаляет саму модель из user_gpt, если это была последняя связь с этой моделью
func (d *DB) RemoveModelFromUser(userID uint32, modelId uint64) error {
	// Проверяем входные значения
	if userID == 0 || modelId == 0 {
		return fmt.Errorf("получены некорректные значения: userID или modelId равны 0")
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	// Начинаем транзакцию для атомарности операций
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback()

	// Проверяем, существует ли связь пользователя с моделью
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM user_models WHERE userID = ? AND ModelId = ?)",
		userID, modelId).Scan(&exists)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при проверке связи пользователя с моделью: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена при проверке связи: %w", err)
		default:
			return fmt.Errorf("ошибка проверки связи пользователя с моделью: %w", err)
		}
	}

	if !exists {
		return fmt.Errorf("связь между пользователем %d и моделью %d не найдена", userID, modelId)
	}

	// Проверяем, была ли эта модель активной
	var wasActive bool
	err = tx.QueryRowContext(ctx,
		"SELECT IsActive FROM user_models WHERE userID = ? AND ModelId = ?",
		userID, modelId).Scan(&wasActive)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при проверке активности модели: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена при проверке активности: %w", err)
		default:
			return fmt.Errorf("ошибка проверки активности модели: %w", err)
		}
	}

	// Удаляем связь между пользователем и моделью
	_, err = tx.ExecContext(ctx,
		"DELETE FROM user_models WHERE userID = ? AND ModelId = ?",
		userID, modelId)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при удалении связи: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена при удалении связи: %w", err)
		default:
			return fmt.Errorf("ошибка удаления связи пользователя с моделью: %w", err)
		}
	}

	// Проверяем, есть ли у этой модели другие связи с пользователями
	var otherUsersCount int
	err = tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_models WHERE ModelId = ?",
		modelId).Scan(&otherUsersCount)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при проверке других связей модели: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена при проверке других связей: %w", err)
		default:
			return fmt.Errorf("ошибка проверки других связей модели: %w", err)
		}
	}

	// Если других связей нет, удаляем саму модель из user_gpt
	if otherUsersCount == 0 {
		_, err = tx.ExecContext(ctx, "DELETE FROM user_gpt WHERE Id = ?", modelId)
		if err != nil {
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				return fmt.Errorf("тайм-аут (%d с) при удалении модели: %w", sqlTimeToCancel, err)
			case errors.Is(err, context.Canceled):
				return fmt.Errorf("операция отменена при удалении модели: %w", err)
			default:
				return fmt.Errorf("ошибка удаления модели: %w", err)
			}
		}
	}

	// Если удалённая модель была активной, нужно активировать другую модель (если есть)
	if wasActive {
		// Получаем первую доступную модель пользователя
		var nextModelId sql.NullInt64
		err = tx.QueryRowContext(ctx,
			"SELECT ModelId FROM user_models WHERE userID = ? LIMIT 1",
			userID).Scan(&nextModelId)

		// Если есть другая модель, делаем её активной
		if err == nil && nextModelId.Valid {
			_, err = tx.ExecContext(ctx,
				"UPDATE user_models SET IsActive = 1 WHERE userID = ? AND ModelId = ?",
				userID, nextModelId.Int64)
			if err != nil {
				return fmt.Errorf("ошибка активации следующей модели: %w", err)
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			// Если других моделей нет - отключаем все каналы пользователя
			// Фиксируем транзакцию перед вызовом DisableAllUserChannel
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("ошибка фиксации транзакции: %w", err)
			}

			// Отключаем все каналы, так как у пользователя больше нет моделей
			if err := d.DisableAllUserChannel(userID); err != nil {
				return fmt.Errorf("ошибка отключения каналов пользователя: %w", err)
			}

			return nil
		}
	}

	// Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}

	return nil
}

func (d *DB) SaveUserModel(
	userID uint32, provider comdom.ProviderType, name, assistantId string, data []byte, def comdom.DefaultProvidersModels, ids json.RawMessage, operator bool) error {
	// Проверяю входные значения
	if userID == 0 || name == "" || assistantId == "" {
		return fmt.Errorf("получены некорректные значения: userID, name или assistantId пусты")
	}
	// Валидация провайдера
	if !provider.IsValid() {
		return fmt.Errorf("некорректный provider: %d (допустимы 1=OpenAI, 2=Mistral, 3=Google)", provider)
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	// Начинаем транзакцию для атомарности операций
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback()

	// ===================================================================
	// Шаг 1: Сохранение/обновление модели в user_gpt
	// ===================================================================

	// Проверяем, существует ли модель для данного пользователя и провайдера
	var existingModelId sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT ug.Id 
		FROM user_gpt ug
		INNER JOIN user_models um ON ug.Id = um.ModelId
		WHERE um.userID = ? AND um.Provider = ?
		LIMIT 1
	`, userID, provider).Scan(&existingModelId)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при проверке существующей модели: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка проверки существующей модели: %w", err)
		}
	}

	var modelId int64
	// A realtime model is optional. Passing numeric zero to MySQL would
	// violate the foreign key; use SQL NULL when no realtime model is set.
	var realtimeModel any
	if def.RealTimeModelID != 0 {
		realtimeModel = def.RealTimeModelID
	}

	if !existingModelId.Valid {
		// ===================================================================
		// Модели нет - создаём новую в user_gpt
		// ===================================================================
		result, err := tx.ExecContext(ctx, `
			INSERT INTO user_gpt (Name, Model, Realtime, Provider, AssistantId, Data, Ids)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, name, def.GeneralModelID, realtimeModel, provider, assistantId, data, ids)

		if err != nil {
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				return fmt.Errorf("тайм-аут (%d с) при создании модели: %w", sqlTimeToCancel, err)
			case errors.Is(err, context.Canceled):
				return fmt.Errorf("операция отменена при создании модели: %w", err)
			default:
				return fmt.Errorf("ошибка создания модели: %w", err)
			}
		}

		// Получаем ID новой записи
		modelId, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("ошибка получения ID новой модели: %w", err)
		}

		// ===================================================================
		// Шаг 2: Создание связи в user_models
		// ===================================================================

		// Проверяем, есть ли у пользователя другие модели
		var modelCount int
		err = tx.QueryRowContext(ctx, `
			SELECT COUNT(*) 
			FROM user_models 
			WHERE userID = ?
		`, userID).Scan(&modelCount)

		if err != nil {
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				return fmt.Errorf("тайм-аут (%d с) при подсчёте моделей: %w", sqlTimeToCancel, err)
			case errors.Is(err, context.Canceled):
				return fmt.Errorf("операция отменена при подсчёте моделей: %w", err)
			default:
				return fmt.Errorf("ошибка подсчёта моделей: %w", err)
			}
		}

		// Если это первая модель - делаем её активной автоматически
		isActive := 0
		if modelCount == 0 {
			isActive = 1
		}

		// Создаём связь в user_models
		_, err = tx.ExecContext(ctx, `
			INSERT INTO user_models (userID, ModelId, Provider, IsActive)
			VALUES (?, ?, ?, ?)
		`, userID, modelId, provider, isActive)

		if err != nil {
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				return fmt.Errorf("тайм-аут (%d с) при создании связи модели: %w", sqlTimeToCancel, err)
			case errors.Is(err, context.Canceled):
				return fmt.Errorf("операция отменена при создании связи: %w", err)
			default:
				return fmt.Errorf("ошибка создания связи модели: %w", err)
			}
		}

	} else {
		// ===================================================================
		// Модель существует - обновляем её в user_gpt
		// ===================================================================
		modelId = existingModelId.Int64

		_, err = tx.ExecContext(ctx, `
			UPDATE user_gpt
			SET Name = ?,
				Model = ?,
				Realtime = ?,
				AssistantId = ?,
				Data = ?,
				Ids = ?
			WHERE Id = ?
		`, name, def.GeneralModelID, realtimeModel, assistantId, data, ids, modelId)

		if err != nil {
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				return fmt.Errorf("тайм-аут (%d с) при обновлении модели: %w", sqlTimeToCancel, err)
			case errors.Is(err, context.Canceled):
				return fmt.Errorf("операция отменена при обновлении модели: %w", err)
			default:
				return fmt.Errorf("ошибка обновления модели: %w", err)
			}
		}
	}

	// ===================================================================
	// Шаг 3: Обновление статуса оператора
	// ===================================================================
	enabledInt := 0
	if operator {
		enabledInt = 1
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE operators
		SET Telegram_enabled = ?,
			Changed = 1,
			Timechange = CURRENT_TIMESTAMP()
		WHERE userID = ?
	`, enabledInt, userID)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при установке статуса оператора: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена при установке статуса оператора: %w", err)
		default:
			return fmt.Errorf("ошибка установки статуса оператора: %w", err)
		}
	}

	// ===================================================================
	// Финал: Фиксируем транзакцию
	// ===================================================================
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}

	return nil
}

// ReadUserModel получает данные модели пользователя и идентификаторы файлов
func (d *DB) ReadUserModel(userID uint32) ([]byte, *comdom.VecIds, error) {
	// Проверяем входное значение
	if userID == 0 {
		return nil, nil, fmt.Errorf("получен некорректный userID")
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.ctx, sqlTimeToCancel*time.Second)
	defer cancel()

	// SQL запрос для получения Data и Ids из user_gpt через активную модель
	query := `
		SELECT TO_BASE64(ug.Data), ug.Ids
		FROM user_models um
		JOIN user_gpt ug ON um.ModelId = ug.Id
		WHERE um.userID = ? AND um.IsActive = 1`

	var base64Data sql.NullString
	var idsJson sql.NullString

	err := d.conn.QueryRowContext(ctx, query, userID).Scan(&base64Data, &idsJson)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, nil, fmt.Errorf("тайм-аут (%d с) при вызове ReadUserModel: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return nil, nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, nil, nil // Модель не найдена, но это не ошибка
		default:
			return nil, nil, fmt.Errorf("ошибка получения данных ReadUserModel: %w", err)
		}
	}

	// Проверяем на пустой результат или null
	if !base64Data.Valid || base64Data.String == "" {
		return nil, nil, nil // Модель не найдена
	}

	// Инициализируем структуру VecIds по умолчанию с пустыми массивами
	vecIds := &comdom.VecIds{
		VectorId: []string{},
		FileIds:  []comdom.Ids{},
	}

	// Проверяем и парсим Ids, если они есть
	if idsJson.Valid && idsJson.String != "" && idsJson.String != "null" {
		if err := json.Unmarshal([]byte(idsJson.String), vecIds); err != nil {
			return nil, nil, fmt.Errorf("ошибка разбора Ids: %w", err)
		}
	}

	// Декодируем base64 данные
	decodedData, err := base64.StdEncoding.DecodeString(base64Data.String)
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка декодирования base64: %w", err)
	}

	return decodedData, vecIds, nil
}

// ReadUserModelByProvider получает сжатые данные модели пользователя по провайдеру
func (d *DB) ReadUserModelByProvider(userID uint32, provider comdom.ProviderType) ([]byte, *comdom.VecIds, error) {
	// Проверяем входные значения
	if userID == 0 {
		return nil, nil, fmt.Errorf("получен некорректный userID")
	}
	if !provider.IsValid() {
		return nil, nil, fmt.Errorf("получен некорректный provider: %d", provider)
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// SQL запрос для получения Data и Ids из user_gpt по провайдеру
	query := `
		SELECT TO_BASE64(ug.Data), ug.Ids
		FROM user_models um
		JOIN user_gpt ug ON um.ModelId = ug.Id
		WHERE um.userID = ? AND um.Provider = ?`

	var base64Data sql.NullString
	var idsJson sql.NullString

	err := d.Conn().QueryRowContext(ctx, query, userID, uint8(provider)).Scan(&base64Data, &idsJson)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, nil, fmt.Errorf("тайм-аут (%d с) при вызове ReadUserModelByProvider: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return nil, nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, nil, nil // Модель не найдена, но это не ошибка
		default:
			return nil, nil, fmt.Errorf("ошибка получения данных ReadUserModelByProvider: %w", err)
		}
	}

	// Проверяем на пустой результат или null
	if !base64Data.Valid || base64Data.String == "" {
		return nil, nil, nil // Модель не найдена
	}

	// Инициализируем структуру VecIds по умолчанию с пустыми массивами
	vecIds := &comdom.VecIds{
		VectorId: []string{},
		FileIds:  []comdom.Ids{},
	}

	// ВАЖНО: Для Google провайдера (provider=3) поле Ids содержит конфигурацию модели,
	// а не file_ids/vector_id, поэтому НЕ парсим его в VecIds
	// Эмбеддинги для Google хранятся в отдельной таблице vector_embeddings
	if provider != comdom.ProviderGoogle {
		// Для OpenAI и Mistral парсим Ids в VecIds (file_ids, vector_id)
		if idsJson.Valid && idsJson.String != "" && idsJson.String != "null" {
			if err := json.Unmarshal([]byte(idsJson.String), vecIds); err != nil {
				return nil, nil, fmt.Errorf("ошибка разбора Ids: %w", err)
			}
		}
	}
	// Для Google провайдера vecIds остаётся с пустыми массивами

	// Декодируем base64 данные
	decodedData, err := base64.StdEncoding.DecodeString(base64Data.String)
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка декодирования base64: %w", err)
	}

	return decodedData, vecIds, nil
}

// DefaultProvidersModels возвращает модель по умолчанию для указанного провайдера
func (d *DB) DefaultProvidersModels(providerName string) (comdom.DefaultProvidersModels, error) {
	// Проверяем входные данные
	if providerName == "" {
		return comdom.DefaultProvidersModels{}, fmt.Errorf("получено пустое имя провайдера")
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	query := `
		SELECT gm.Id   AS GptModelId,
    		gm.Name AS GptModelName,
    		rm.Id   AS RealtimeModelId,
    		rm.Name AS RealtimeModelName
		FROM gpt_models gm
    	INNER JOIN model_providers mp ON gm.Provider = mp.Id
    	INNER JOIN realtime_models rm ON rm.Provider = mp.Id
    	WHERE mp.Name = ?
    		AND gm.IsDefault = 1
    	LIMIT 1;
	`

	var (
		GptModelId, RealtimeModelId     uint
		GptModelName, RealtimeModelName string
	)

	err := d.Conn().QueryRowContext(ctx, query, providerName).Scan(&GptModelId, &GptModelName, &RealtimeModelId, &RealtimeModelName)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return comdom.DefaultProvidersModels{}, fmt.Errorf("тайм-аут (%d с) при получении модели по умолчанию для провайдера %s: %w", sqlTimeToCancel, providerName, err)
		case errors.Is(err, context.Canceled):
			return comdom.DefaultProvidersModels{}, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return comdom.DefaultProvidersModels{}, fmt.Errorf("модель по умолчанию для провайдера %s не найдена", providerName)
		default:
			return comdom.DefaultProvidersModels{}, fmt.Errorf("ошибка выполнения запроса: %w", err)
		}
	}

	return comdom.DefaultProvidersModels{
		GeneralModelID:    GptModelId,
		GeneralModelName:  GptModelName,
		RealTimeModelID:   RealtimeModelId,
		RealTimeModelName: RealtimeModelName,
	}, nil
}

// GetActiveModel получает активную модель пользователя
func (d *DB) GetActiveModel(userID uint32) (*comdom.UserModelRecord, error) {
	// Проверяем входное значение
	if userID == 0 {
		return nil, fmt.Errorf("получен некорректный userID")
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// SQL запрос для получения активной модели
	query := `
		SELECT 
			um.Id,
			ug.AssistantId,
			um.Provider,
			um.IsActive,
			ug.Ids,
			gm.Id,
			gm.Name,
			rm.Id,
			rm.Name
		FROM user_models um
		JOIN user_gpt ug ON um.ModelId = ug.Id
		LEFT JOIN gpt_models gm ON gm.Id = ug.Model
		LEFT JOIN realtime_models rm ON rm.Id = ug.Realtime
		WHERE um.userID = ? AND um.IsActive = 1
		LIMIT 1`

	var modelId uint64
	var assistId string
	var provider uint8
	var isActive bool
	var idsJson sql.NullString
	var modelName sql.NullString
	var modelNameId sql.NullInt64
	var realtimeName sql.NullString
	var realtimeNameId sql.NullInt64

	err := d.Conn().QueryRowContext(ctx, query, userID).Scan(
		&modelId,
		&assistId,
		&provider,
		&isActive,
		&idsJson,
		&modelNameId,
		&modelName,
		&realtimeNameId,
		&realtimeName,
	)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при вызове GetActiveModel: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, nil // Активная модель не найдена
		default:
			return nil, fmt.Errorf("ошибка получения активной модели: %w", err)
		}
	}

	// Создаем запись модели
	record := &comdom.UserModelRecord{
		ModelId:  modelId,
		AssistId: assistId,
		Provider: comdom.ProviderType(provider),
		IsActive: isActive,
		FileIds:  []comdom.Ids{},
	}
	if modelName.Valid && modelNameId.Valid {
		record.GptType = &comdom.GptType{
			ID:   uint(modelNameId.Int64),
			Name: modelName.String,
		}
	}
	if realtimeName.Valid && realtimeNameId.Valid {
		record.Realtime = &comdom.Realtime{
			ID:   uint(realtimeNameId.Int64),
			Name: realtimeName.String,
		}
	}

	// Парсим JSON с Ids
	if idsJson.Valid && idsJson.String != "" && idsJson.String != "null" {
		record.AllIds = []byte(idsJson.String)

		var vecIds comdom.VecIds
		if err := json.Unmarshal([]byte(idsJson.String), &vecIds); err != nil {
			return nil, fmt.Errorf("ошибка разбора Ids: %w", err)
		}
		record.FileIds = vecIds.FileIds
	}

	return record, nil
}

// GetModelByProvider получает АКТИВНУЮ модель пользователя по провайдеру
// Если модель не активна - возвращает nil
func (d *DB) GetModelByProvider(userID uint32, provider comdom.ProviderType) (*comdom.UserModelRecord, error) {
	// Проверяем входные значения
	if userID == 0 {
		return nil, fmt.Errorf("получен некорректный userID")
	}
	if !provider.IsValid() {
		return nil, fmt.Errorf("получен некорректный provider: %d", provider)
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// SQL запрос для получения модели по провайдеру
	query := `
		SELECT 
			um.ModelId,
			ug.AssistantId,
			um.Provider,
			um.IsActive,
			ug.Ids,
			gm.Id,
			gm.Name,
			rm.Id,
			rm.Name
		FROM user_models um
		INNER JOIN user_gpt ug ON um.ModelId = ug.Id
		LEFT JOIN gpt_models gm ON gm.Id = ug.Model
		LEFT JOIN realtime_models rm ON rm.Id = ug.Realtime
		WHERE um.userID = ? 
			AND um.Provider = ?
		LIMIT 1`

	var modelId uint64
	var assistId string
	var providerDb uint8
	var isActive bool
	var idsJson sql.NullString
	var modelName sql.NullString
	var modelNameId sql.NullInt64
	var realtimeName sql.NullString
	var realtimeNameId sql.NullInt64

	err := d.Conn().QueryRowContext(ctx, query, userID, uint8(provider)).Scan(
		&modelId,
		&assistId,
		&providerDb,
		&isActive,
		&idsJson,
		&modelNameId,
		&modelName,
		&realtimeNameId,
		&realtimeName,
	)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при вызове GetModelByProvider: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, nil // Модель не найдена
		default:
			return nil, fmt.Errorf("ошибка получения модели по провайдеру: %w", err)
		}
	}

	// Создаем запись модели
	record := &comdom.UserModelRecord{
		ModelId:  modelId,
		AssistId: assistId,
		Provider: comdom.ProviderType(providerDb),
		IsActive: isActive,
		FileIds:  []comdom.Ids{},
	}
	if modelName.Valid && modelNameId.Valid {
		record.GptType = &comdom.GptType{
			ID:   uint(modelNameId.Int64),
			Name: modelName.String,
		}
	}
	if realtimeName.Valid && realtimeNameId.Valid {
		record.Realtime = &comdom.Realtime{
			ID:   uint(realtimeNameId.Int64),
			Name: realtimeName.String,
		}
	}

	// Парсим JSON с Ids
	if idsJson.Valid && idsJson.String != "" && idsJson.String != "null" {
		record.AllIds = []byte(idsJson.String)

		var vecIds comdom.VecIds
		if err := json.Unmarshal([]byte(idsJson.String), &vecIds); err != nil {
			return nil, fmt.Errorf("ошибка разбора Ids: %w", err)
		}
		record.FileIds = vecIds.FileIds
	}

	return record, nil
}

// GetModelByProviderAnyStatus получает модель пользователя по провайдеру НЕЗАВИСИМО от статуса активности
// В отличие от GetModelByProvider, эта функция не требует IsActive = 1
// Используется для обновления неактивных моделей
func (d *DB) GetModelByProviderAnyStatus(userID uint32, provider comdom.ProviderType) (*comdom.UserModelRecord, error) {
	// Проверяем входные значения
	if userID == 0 {
		return nil, fmt.Errorf("получен некорректный userID")
	}
	if !provider.IsValid() {
		return nil, fmt.Errorf("получен некорректный provider: %d", provider)
	}

	// Дочерний контекст с тайм-аутом на операцию
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// SQL запрос - БЕЗ условия IsActive = 1
	query := `
		SELECT 
			um.ModelId,
			ug.AssistantId,
			um.Provider,
			um.IsActive,
			ug.Ids,
			gm.Id,
			gm.Name,
			rm.Id,
			rm.Name
		FROM user_models um
		INNER JOIN user_gpt ug ON um.ModelId = ug.Id
		LEFT JOIN gpt_models gm ON gm.Id = ug.Model
		LEFT JOIN realtime_models rm ON rm.Id = ug.Realtime
		WHERE um.userID = ? 
			AND um.Provider = ?
		LIMIT 1`

	var modelId uint64
	var assistId string
	var providerDb uint8
	var isActive bool
	var idsJson sql.NullString
	var modelName sql.NullString
	var modelNameId sql.NullInt64
	var realtimeName sql.NullString
	var realtimeNameId sql.NullInt64

	err := d.Conn().QueryRowContext(ctx, query, userID, uint8(provider)).Scan(
		&modelId,
		&assistId,
		&providerDb,
		&isActive,
		&idsJson,
		&modelNameId,
		&modelName,
		&realtimeNameId,
		&realtimeName,
	)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("тайм-аут (%d с) при вызове GetModelByProviderAnyStatus: %w", sqlTimeToCancel, err)
		case errors.Is(err, context.Canceled):
			return nil, fmt.Errorf("операция отменена: %w", err)
		case errors.Is(err, sql.ErrNoRows):
			return nil, nil // Модель не найдена
		default:
			return nil, fmt.Errorf("ошибка получения модели по провайдеру: %w", err)
		}
	}

	// Создаем запись модели
	record := &comdom.UserModelRecord{
		ModelId:  modelId,
		AssistId: assistId,
		Provider: comdom.ProviderType(providerDb),
		IsActive: isActive,
		FileIds:  []comdom.Ids{},
	}
	if modelName.Valid && modelNameId.Valid {
		record.GptType = &comdom.GptType{
			ID:   uint(modelNameId.Int64),
			Name: modelName.String,
		}
	}
	if realtimeName.Valid && realtimeNameId.Valid {
		record.Realtime = &comdom.Realtime{
			ID:   uint(realtimeNameId.Int64),
			Name: realtimeName.String,
		}
	}

	// Парсим JSON с Ids
	if idsJson.Valid && idsJson.String != "" && idsJson.String != "null" {
		record.AllIds = []byte(idsJson.String)

		var vecIds comdom.VecIds
		if err := json.Unmarshal([]byte(idsJson.String), &vecIds); err != nil {
			return nil, fmt.Errorf("ошибка разбора Ids: %w", err)
		}
		record.FileIds = vecIds.FileIds
	}

	return record, nil
}

// SetActiveModel переключает активную модель пользователя
// Параметры:
//   - userID: ID пользователя
//   - modelId: ID записи из таблицы user_models
//
// Функция снимает IsActive с других моделей пользователя в этой же транзакции
func (d *DB) SetActiveModel(userID uint32, modelId uint64) error {
	if userID == 0 {
		return fmt.Errorf("получен пустой userID")
	}

	if modelId == 0 {
		return fmt.Errorf("получен пустой modelId")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	// Начинаем транзакцию
	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback()

	// Сначала снимаем IsActive со всех активных моделей этого пользователя
	_, err = tx.ExecContext(ctx,
		"UPDATE user_models SET IsActive = 0 WHERE userID = ? AND IsActive = 1",
		userID)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при деактивации старых моделей: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка деактивации старых моделей: %w", err)
		}
	}

	// Обновляем IsActive для указанной модели
	result, err := tx.ExecContext(ctx,
		"UPDATE user_models SET IsActive = 1 WHERE Id = ? AND userID = ?",
		modelId, userID)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при переключении активной модели: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка переключения активной модели: %w", err)
		}
	}

	// Проверяем, была ли обновлена хотя бы одна строка
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ошибка получения количества обновленных строк: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("модель с Id=%d для пользователя %d не найдена", modelId, userID)
	}

	// Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}

	return nil
}

// SetActiveModelByProvider переключает активную модель пользователя для указанного провайдера
// Параметры:
//   - userID: ID пользователя
//   - provider: тип провайдера (ProviderOpenAI, ProviderMistral, ...)
//
// Функция снимает IsActive с других моделей пользователя в этой же транзакции
func (d *DB) SetActiveModelByProvider(userID uint32, provider comdom.ProviderType) error {
	if userID == 0 {
		return fmt.Errorf("получен пустой userID")
	}

	ctx, cancel := context.WithTimeout(d.Context(), mode.GetSQLTimeToCancel())
	defer cancel()

	// Начинаем транзакцию
	tx, err := d.Conn().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback()

	// Сначала снимаем IsActive со ВСЕХ активных моделей пользователя (любого провайдера)
	_, err = tx.ExecContext(ctx,
		`UPDATE user_models 
		SET IsActive = 0 
		WHERE userID = ? AND IsActive = 1`,
		userID)

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при деактивации старой модели: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка деактивации старой модели: %w", err)
		}
	}

	// Обновляем IsActive для пользовательской модели указанного провайдера
	result, err := tx.ExecContext(ctx,
		`UPDATE user_models 
		SET IsActive = 1 
		WHERE userID = ? AND Provider = ? 
		ORDER BY CreatedAt DESC 
		LIMIT 1`,
		userID, uint8(provider))

	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("тайм-аут (%d с) при переключении активной модели: %w", mode.GetSQLTimeToCancel(), err)
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("операция отменена: %w", err)
		default:
			return fmt.Errorf("ошибка переключения активной модели: %w", err)
		}
	}

	// Проверяем, была ли обновлена хотя бы одна строка
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ошибка получения количества обновленных строк: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("пользовательская модель провайдера %s для пользователя %d не найдена", provider.String(), userID)
	}

	// Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}

	return nil
}

// UpdateUserGPT обновляет поле Ids (AllIds) в таблице user_gpt
// Используется для обновления информации о файлах и векторных хранилищах/библиотеках
func (d *DB) UpdateUserGPT(userID uint32, modelId uint64, assistId string, allIds []byte) error {
	if userID == 0 {
		return fmt.Errorf("получен пустой userID")
	}
	if modelId == 0 {
		return fmt.Errorf("получен пустой modelId")
	}

	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	defer cancel()

	// Подготавливаем значение для БД
	// Если allIds == nil, то сохраняем SQL NULL, иначе строку
	var idsValue any
	if allIds == nil || len(allIds) == 0 {
		idsValue = nil // SQL NULL
	} else {
		idsValue = string(allIds)
	}

	// Обновляем поле Ids в user_gpt
	_, err := d.Conn().ExecContext(ctx, `
		UPDATE user_gpt
		SET Ids = ?
		WHERE Id = ? AND AssistantId = ?
	`, idsValue, modelId, assistId)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("тайм-аут (%d с) при обновлении user_gpt: %w", sqlTimeToCancel, err)
		}
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("операция отменена: %w", err)
		}
		return fmt.Errorf("ошибка обновления user_gpt: %w", err)
	}

	return nil
}
