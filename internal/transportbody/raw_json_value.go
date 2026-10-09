package transportbody

import (
	"bufio"
	"encoding/json"
	"io"
)

type rawJSONValueReader struct {
	reader  *bufio.Reader
	content []byte
}

type jsonStringState struct {
	inString bool
	escaped  bool
}

func (state *jsonStringState) consume(value byte) bool {
	if !state.inString {
		return false
	}
	switch {
	case state.escaped:
		state.escaped = false
	case value == '\\':
		state.escaped = true
	case value == '"':
		state.inString = false
	}
	return true
}

func readRawJSONValue(reader *bufio.Reader, first byte) ([]byte, error) {
	raw := rawJSONValueReader{reader: reader, content: []byte{first}}
	var err error
	switch first {
	case '{', '[':
		err = raw.readContainer()
	case '"':
		err = raw.readString()
	default:
		err = raw.readPrimitive()
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw.content) {
		return nil, ErrInvalidJSON
	}
	return raw.content, nil
}

func (raw *rawJSONValueReader) append(value byte) error {
	if int64(len(raw.content)) >= maxChatTemplateKwargsBytes {
		return ErrChatTemplateKwargsTooLarge
	}
	raw.content = append(raw.content, value)
	return nil
}

func (raw *rawJSONValueReader) next() (byte, error) {
	value, err := raw.reader.ReadByte()
	if err != nil {
		return 0, rawJSONReadError(err)
	}
	return value, raw.append(value)
}

func (raw *rawJSONValueReader) readContainer() error {
	depth := 1
	var stringState jsonStringState
	for depth > 0 {
		value, err := raw.next()
		if err != nil {
			return err
		}
		if stringState.consume(value) {
			continue
		}
		switch value {
		case '"':
			stringState.inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return nil
}

func (raw *rawJSONValueReader) readString() error {
	state := jsonStringState{inString: true}
	for state.inString {
		value, err := raw.next()
		if err != nil {
			return err
		}
		state.consume(value)
	}
	return nil
}

func (raw *rawJSONValueReader) readPrimitive() error {
	for {
		value, err := raw.reader.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if isJSONDelimiter(value) {
			return raw.reader.UnreadByte()
		}
		if err := raw.append(value); err != nil {
			return err
		}
	}
}
