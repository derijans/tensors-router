package catalog

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

type LenientInt int

func (value *LenientInt) UnmarshalJSON(raw []byte) error {
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		*value = LenientInt(number)
		return nil
	}
	var quoted string
	if err := json.Unmarshal(raw, &quoted); err != nil {
		return notAWholeNumber(string(raw))
	}
	trimmed := strings.TrimSpace(quoted)
	if trimmed == "" {
		*value = 0
		return nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return notAWholeNumber("string " + strconv.Quote(quoted))
	}
	*value = LenientInt(parsed)
	return nil
}

func notAWholeNumber(value string) error {
	return &json.UnmarshalTypeError{Value: value, Type: reflect.TypeFor[int]()}
}
