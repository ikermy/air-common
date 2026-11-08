package migrate

import (
	"fmt"
	"strings"
)

// splitStatements splits a SQL migration into executable statements. It
// understands single/double/backtick quotes and --, #, /* */ comments so
// semicolons inside literals or comments are not mistaken for separators.
func splitStatements(src string) ([]string, error) {
	var (
		statements []string
		current    strings.Builder
		inSingle   bool
		inDouble   bool
		inBacktick bool
		inLine     bool
		inBlock    bool
	)
	runes := []rune(src)

	for i := 0; i < len(runes); i++ {
		c := runes[i]
		var next rune
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		switch {
		case inLine:
			if c == '\n' {
				inLine = false
				current.WriteRune(c)
			}
			continue

		case inBlock:
			if c == '*' && next == '/' {
				inBlock = false
				i++
			}
			continue

		case inSingle:
			current.WriteRune(c)
			if c == '\\' && i+1 < len(runes) {
				i++
				current.WriteRune(runes[i])
				continue
			}
			if c == '\'' {
				inSingle = false
			}
			continue

		case inDouble:
			current.WriteRune(c)
			if c == '\\' && i+1 < len(runes) {
				i++
				current.WriteRune(runes[i])
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue

		case inBacktick:
			current.WriteRune(c)
			if c == '`' {
				inBacktick = false
			}
			continue
		}

		// Not inside a literal or comment.
		if c == '-' && next == '-' {
			inLine = true
			i++
			continue
		}
		if c == '#' {
			inLine = true
			continue
		}
		if c == '/' && next == '*' {
			inBlock = true
			i++
			continue
		}
		if c == '\'' {
			inSingle = true
			current.WriteRune(c)
			continue
		}
		if c == '"' {
			inDouble = true
			current.WriteRune(c)
			continue
		}
		if c == '`' {
			inBacktick = true
			current.WriteRune(c)
			continue
		}
		if c == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}
		current.WriteRune(c)
	}

	switch {
	case inSingle:
		return nil, fmt.Errorf("unterminated single-quoted string")
	case inDouble:
		return nil, fmt.Errorf("unterminated double-quoted string")
	case inBacktick:
		return nil, fmt.Errorf("unterminated backtick identifier")
	case inBlock:
		return nil, fmt.Errorf("unterminated block comment")
	}

	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements, nil
}
