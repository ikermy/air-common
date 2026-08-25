package comdb

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ikermy/air-common/pkg/crypto"
)

// processReadDialogResult выполняет расшифровку и нормализацию данных диалога.
func (d *DB) processReadDialogResult(ctx context.Context, dialogId uint64, raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}

	dataField, ok := obj["Data"]
	if !ok {
		return raw
	}

	processedData := dataField

	// 1. Пробуем расшифровать, если поле Data является строкой с префиксом $mk$
	var dataStr string
	if err := json.Unmarshal(dataField, &dataStr); err == nil && crypto.IsEncryptedWithMasterKey(dataStr) {
		if d.MasterKeyResolver != nil {
			var userId uint32
			if err := d.Conn().QueryRowContext(ctx,
				"SELECT `User` FROM dialogs WHERE Id = ? LIMIT 1", dialogId).Scan(&userId); err == nil {
				if mk, ok := d.MasterKeyResolver(userId); ok {
					if plain, err := crypto.DecryptFieldWithMasterKey(mk, dataStr); err == nil {
						processedData = json.RawMessage(plain)
					}
				}
			}
		}
	}

	// 2. Всегда нормализуем массив (превращаем строки-JSON в объекты)
	obj["Data"] = d.normalizeDataArray(processedData)

	result, _ := json.Marshal(obj)
	return result
}

// normalizeDataArray гарантирует, что каждый элемент в массиве является JSON-объектом, а не строкой.
func (d *DB) normalizeDataArray(data json.RawMessage) json.RawMessage {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return data
	}

	changed := false
	for i, item := range arr {
		var s string
		// Если элемент — это JSON-строка (начинается на "), пробуем её распарсить
		if len(item) > 0 && item[0] == '"' {
			if err := json.Unmarshal(item, &s); err == nil {
				trimmed := strings.TrimSpace(s)
				// Проверяем, что внутри действительно объект или массив
				if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
					(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
					arr[i] = json.RawMessage(trimmed)
					changed = true
				}
			}
		}
	}

	if !changed {
		return data
	}

	newBytes, err := json.Marshal(arr)
	if err != nil {
		return data
	}
	return json.RawMessage(newBytes)
}
