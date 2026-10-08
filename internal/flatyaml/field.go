package flatyaml

import (
	"strconv"
	"time"
)

type Field struct {
	assign func(value string) error
	list   *[]string
}

func Scalar(assign func(value string) error) Field {
	return Field{assign: assign}
}

func String(target *string) Field {
	return Scalar(func(value string) error {
		*target = value
		return nil
	})
}

func Bool(target *bool) Field {
	return Scalar(func(value string) error {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		*target = parsed
		return nil
	})
}

func Int(target *int) Field {
	return Scalar(func(value string) error {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		*target = parsed
		return nil
	})
}

func Int64(target *int64) Field {
	return Scalar(func(value string) error {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		*target = parsed
		return nil
	})
}

func Duration(target *time.Duration) Field {
	return Scalar(func(value string) error {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*target = parsed
		return nil
	})
}

func StringList(target *[]string) Field {
	return Field{list: target}
}

func (field Field) Marking(present *bool) Field {
	assign := field.assign
	field.assign = func(value string) error {
		if err := assign(value); err != nil {
			return err
		}
		*present = true
		return nil
	}
	return field
}

func (field Field) isList() bool {
	return field.list != nil
}
