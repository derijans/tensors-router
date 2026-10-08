package flatyaml

import (
	"fmt"
	"strings"
)

type decoder struct {
	dialect Dialect
	schema  Schema
	section string
	list    *[]string
}

func Decode(content []byte, dialect Dialect, schema Schema) error {
	state := decoder{dialect: dialect, schema: schema}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for index, rawLine := range lines {
		if err := state.decodeLine(rawLine); err != nil {
			return fmt.Errorf("line %d: %w", index+1, err)
		}
	}
	return nil
}

func (state *decoder) decodeLine(rawLine string) error {
	trimmed := strings.TrimSpace(rawLine)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}
	if strings.Contains(rawLine, "\t") {
		return fmt.Errorf("tabs are not supported")
	}
	line := strings.TrimSpace(state.dialect.stripComment(trimmed))
	if line == "" {
		return nil
	}
	if !strings.HasPrefix(rawLine, " ") && strings.HasSuffix(line, ":") {
		state.section = strings.TrimSuffix(line, ":")
		state.list = nil
		return nil
	}
	if state.section == "" {
		return fmt.Errorf("expected a section")
	}
	if state.dialect.supportsLists() && strings.HasPrefix(line, "- ") {
		return state.appendListItem(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
	}
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return fmt.Errorf("expected key value")
	}
	state.list = nil
	return state.assign(strings.TrimSpace(key), strings.TrimSpace(value))
}

func (state *decoder) appendListItem(rawItem string) error {
	if state.list == nil {
		return fmt.Errorf("list item without list key")
	}
	item, err := state.dialect.Scalar(rawItem)
	if err != nil {
		return err
	}
	*state.list = append(*state.list, item)
	return nil
}

func (state *decoder) assign(key string, value string) error {
	if state.dialect.supportsLists() && value == "" {
		return state.startBlockList(key)
	}
	if state.dialect.supportsLists() && strings.HasPrefix(value, "[") {
		values, err := state.dialect.flowList(value)
		if err != nil {
			return err
		}
		return state.assignList(key, values)
	}
	scalar, err := state.dialect.Scalar(value)
	if err != nil {
		return err
	}
	field, err := state.field(key)
	if err != nil {
		return err
	}
	if field.isList() {
		return unknownKey(state.section, key)
	}
	return field.assign(scalar)
}

func (state *decoder) startBlockList(key string) error {
	target, err := state.listTarget(key)
	if err != nil {
		return err
	}
	*target = []string{}
	state.list = target
	return nil
}

func (state *decoder) assignList(key string, values []string) error {
	target, err := state.listTarget(key)
	if err != nil {
		return err
	}
	*target = values
	return nil
}

func (state *decoder) listTarget(key string) (*[]string, error) {
	field, err := state.field(key)
	if err != nil {
		return nil, err
	}
	if !field.isList() {
		return nil, unknownKey(state.section, key)
	}
	return field.list, nil
}

func (state *decoder) field(key string) (Field, error) {
	return state.schema.field(state.section, key, state.dialect.NamesUnknownSections)
}
