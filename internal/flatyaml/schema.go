package flatyaml

import "fmt"

type Schema map[string]Section

type Section struct {
	Fields  Fields
	Dynamic func(key string) (Field, bool)
}

type Fields map[string]Field

func (schema Schema) field(section string, key string, namesUnknownSections bool) (Field, error) {
	fields, sectionKnown := schema[section]
	if !sectionKnown && namesUnknownSections {
		return Field{}, fmt.Errorf("unknown section %s", section)
	}
	if field, ok := fields.Fields[key]; ok {
		return field, nil
	}
	if fields.Dynamic != nil {
		if field, ok := fields.Dynamic(key); ok {
			return field, nil
		}
	}
	return Field{}, unknownKey(section, key)
}

func unknownKey(section string, key string) error {
	return fmt.Errorf("unknown key %s.%s", section, key)
}
