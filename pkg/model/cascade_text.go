package model

import "strings"

// Режимы cascadeTextExtractor.
const (
	cascadeTextUndecided = iota
	cascadeTextPlain
	cascadeTextJSONMessage
	cascadeTextDone
)

// cascadeTextExtractor превращает поток сырых delta-чанков активного LLM в
// чистый текст для озвучки/событий realtime-каскада.
//
// Провайдеры отдают в onDelta не только текст: Mistral Conversations шлёт JSON-
// конверт {"message":"..."}, Google — финальный {"type":"token_usage",...},
// возможны function_call-кадры. Всё это нельзя ни произносить, ни показывать.
// Поэтому:
//   - если поток начинается с "{", считаем его JSON-конвертом и извлекаем
//     значение поля "message" (инкрементально, с учётом частичных чанков);
//   - иначе — обычный текст, а JSON-кадры управления игнорируются.
type cascadeTextExtractor struct {
	raw     strings.Builder
	mode    int
	emitted int
}

// Push обрабатывает очередную дельту и возвращает новый (ещё не отданный) текст.
func (e *cascadeTextExtractor) Push(delta string) string {
	if e == nil || delta == "" || e.mode == cascadeTextDone {
		return ""
	}

	// Обычный текст (провайдер отдаёт чистые дельты): управляющие JSON-кадры
	// (token_usage, function_call) не озвучиваем.
	if e.mode == cascadeTextPlain {
		if strings.HasPrefix(strings.TrimSpace(delta), "{") {
			return ""
		}
		return delta
	}

	e.raw.WriteString(delta)
	if e.mode == cascadeTextJSONMessage {
		return e.drainJSONMessage(false)
	}
	buf := e.raw.String()

	// Детекция не привязана к первому символу: Mistral Conversations может
	// отдавать конверт с SSE-обвязкой/несколькими JSON-объектами, поэтому как и
	// нативный extractor ищем ключ "message" в любом месте буфера.
	if !strings.Contains(buf, `"message"`) {
		if e.mode == cascadeTextUndecided && !strings.Contains(buf, "{") {
			e.mode = cascadeTextPlain
			e.raw.Reset()
			return buf
		}
		return ""
	}

	e.mode = cascadeTextJSONMessage
	return e.drainJSONMessage(false)
}

// Flush возвращает остаток текста, если JSON-конверт не был закрыт к концу стрима.
func (e *cascadeTextExtractor) Flush() string {
	if e == nil || e.mode != cascadeTextJSONMessage {
		return ""
	}
	return e.drainJSONMessage(true)
}

func (e *cascadeTextExtractor) drainJSONMessage(force bool) string {
	buf := e.raw.String()

	if e.emitted == 0 {
		marker := strings.Index(buf, `"message"`)
		if marker < 0 {
			return ""
		}
		rest := buf[marker+len(`"message"`):]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return ""
		}
		rest = strings.TrimLeft(rest[colon+1:], " \t\r\n")
		if rest == "" {
			return ""
		}
		if rest[0] != '"' {
			// message не строка — озвучивать нечего.
			e.mode = cascadeTextDone
			return ""
		}
		e.raw.Reset()
		e.raw.WriteString(rest[1:])
		buf = e.raw.String()
	}

	var out strings.Builder
	i := 0
	for i < len(buf) {
		ch := buf[i]
		switch {
		case ch == '\\':
			if i+1 >= len(buf) {
				if !force {
					goto done
				}
				i++
				continue
			}
			out.WriteByte(decodeCascadeEscape(buf[i+1]))
			i += 2
		case ch == '"':
			e.mode = cascadeTextDone
			i++
			goto done
		default:
			out.WriteByte(ch)
			i++
		}
	}
done:
	text := out.String()
	if len(text) <= e.emitted {
		return ""
	}
	increment := text[e.emitted:]
	e.emitted = len(text)
	return increment
}

func decodeCascadeEscape(b byte) byte {
	switch b {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	default:
		return b
	}
}
