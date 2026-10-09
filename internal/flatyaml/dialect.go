package flatyaml

import (
	"fmt"
	"strconv"
	"strings"
)

type Dialect struct {
	StripComment         func(line string) string
	Scalar               func(value string) (string, error)
	SplitListItems       func(body string) []string
	NamesUnknownSections bool
}

func (dialect Dialect) supportsLists() bool {
	return dialect.SplitListItems != nil
}

func (dialect Dialect) stripComment(line string) string {
	if dialect.StripComment == nil {
		return line
	}
	return dialect.StripComment(line)
}

func (dialect Dialect) flowList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "[]" {
		return []string{}, nil
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("invalid list")
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if body == "" {
		return []string{}, nil
	}
	items := dialect.SplitListItems(body)
	values := make([]string, 0, len(items))
	for _, item := range items {
		value, err := dialect.Scalar(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func UnquoteScalar(value string) (string, error) {
	if strings.HasPrefix(value, "'") {
		if !strings.HasSuffix(value, "'") || len(value) < 2 {
			return "", fmt.Errorf("invalid quoted string")
		}
		return strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'"), nil
	}
	if strings.HasPrefix(value, "\"") {
		return strconv.Unquote(value)
	}
	return value, nil
}

func SplitOnEveryComma(body string) []string {
	return strings.Split(body, ",")
}

func SplitOnUnquotedCommas(body string) []string {
	var items []string
	var current strings.Builder
	var quotes quoteTracker
	for _, char := range body {
		if !quotes.consume(char) && char == ',' {
			items = append(items, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(char)
	}
	return append(items, current.String())
}

type quoteTracker struct {
	quote   rune
	escaped bool
}

func (tracker *quoteTracker) consume(char rune) (quoted bool) {
	switch {
	case tracker.escaped:
		tracker.escaped = false
	case tracker.quote == '"' && char == '\\':
		tracker.escaped = true
	case tracker.quote != 0:
		if char == tracker.quote {
			tracker.quote = 0
		}
	case char == '\'' || char == '"':
		tracker.quote = char
	default:
		return false
	}
	return true
}

func StripTrailingComment(line string) string {
	inSingle := false
	inDouble := false
	for index := 0; index < len(line); index++ {
		character := line[index]
		switch {
		case inDouble && character == '\\':
			index++
		case character == '"' && !inSingle:
			inDouble = !inDouble
		case character == '\'' && !inDouble:
			inSingle = !inSingle
		case character == '#' && !inSingle && !inDouble:
			if index == 0 || line[index-1] == ' ' || line[index-1] == '\t' {
				return strings.TrimRight(line[:index], " \t")
			}
		}
	}
	return line
}
