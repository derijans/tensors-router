package transportbody

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	selectorValueLimit = 1024
	maxJSONNesting     = 1024
)

const (
	PathModel              = "model"
	PathImageModel         = "sd_model_checkpoint"
	PathOverrideImageModel = "override_settings.sd_model_checkpoint"
	objectKeyState         = iota
	objectColonState
	objectValueState
	objectCommaState
	arrayValueState
	arrayCommaState
)

type StringReplacement struct {
	From string
	To   string
}

type JSONRewrite struct {
	Replacements map[string]StringReplacement
	EscapeHTML   bool
}

type JSONFields struct {
	Model              string
	ModelSet           bool
	ImageModel         string
	ImageModelSet      bool
	OverrideImageModel string
	OverrideModelSet   bool
	Width              int64
	WidthSet           bool
	Height             int64
	HeightSet          bool
	Count              int64
	CountSet           bool
	BatchSize          int64
	BatchSizeSet       bool
	BatchCount         int64
	BatchCountSet      bool
}

func InspectJSON(body Body) (JSONFields, error) {
	if body == nil || !body.Replayable() {
		return JSONFields{}, ErrSelectorRequired
	}
	attempt, err := body.OpenAttempt()
	if err != nil {
		return JSONFields{}, err
	}
	defer attempt.Close()
	return processJSON(attempt, io.Discard, JSONRewrite{})
}

func TransformJSON(body Body, rewrite JSONRewrite) Body {
	return Transform(body, 0, false, func(source io.ReadCloser) (io.ReadCloser, error) {
		return NewJSONTransformReadCloser(source, rewrite), nil
	})
}

func NewJSONTransformReadCloser(source io.ReadCloser, rewrite JSONRewrite) io.ReadCloser {
	return newLazyTransformReadCloser(source, func(reader io.Reader, writer io.Writer) error {
		_, err := processJSON(reader, writer, rewrite)
		return err
	})
}

type jsonContainer struct {
	kind     byte
	state    int
	key      string
	root     bool
	override bool
}

type jsonProcessor struct {
	reader   *bufio.Reader
	writer   *bufio.Writer
	rewrite  JSONRewrite
	fields   JSONFields
	stack    []jsonContainer
	started  bool
	complete bool
}

func processJSON(source io.Reader, destination io.Writer, rewrite JSONRewrite) (JSONFields, error) {
	processor := &jsonProcessor{
		reader:  bufio.NewReaderSize(source, 32*1024),
		writer:  bufio.NewWriterSize(destination, 32*1024),
		rewrite: rewrite,
	}
	if rewrite.Replacements == nil {
		processor.rewrite.Replacements = map[string]StringReplacement{}
	}
	err := processor.run()
	flushErr := processor.writer.Flush()
	if err != nil {
		return processor.fields, err
	}
	return processor.fields, flushErr
}

func (processor *jsonProcessor) run() error {
	for {
		value, err := processor.reader.ReadByte()
		if err == io.EOF {
			if !processor.complete || len(processor.stack) != 0 {
				return ErrInvalidJSON
			}
			return nil
		}
		if err != nil {
			return err
		}
		if isJSONWhitespace(value) {
			if err := processor.writer.WriteByte(value); err != nil {
				return err
			}
			continue
		}
		if len(processor.stack) == 0 {
			if processor.complete || processor.started || value != '{' {
				return ErrInvalidJSON
			}
			processor.started = true
			if err := processor.writer.WriteByte(value); err != nil {
				return err
			}
			processor.stack = append(processor.stack, jsonContainer{kind: '{', state: objectKeyState, root: true})
			continue
		}
		if err := processor.consume(value); err != nil {
			return err
		}
	}
}

func (processor *jsonProcessor) consume(value byte) error {
	index := len(processor.stack) - 1
	container := &processor.stack[index]
	if container.kind == '{' {
		switch container.state {
		case objectKeyState:
			if value == '}' {
				return processor.closeContainer(value)
			}
			if value != '"' {
				return ErrInvalidJSON
			}
			if err := processor.writer.WriteByte(value); err != nil {
				return err
			}
			raw, overflow, err := processor.copyString(selectorValueLimit, processor.rewrite.EscapeHTML)
			if err != nil {
				return err
			}
			container = &processor.stack[index]
			container.key = ""
			if !overflow {
				container.key, err = decodeJSONString(raw)
				if err != nil {
					return ErrInvalidJSON
				}
			}
			container.state = objectColonState
			return nil
		case objectColonState:
			if value != ':' {
				return ErrInvalidJSON
			}
			container.state = objectValueState
			return processor.writer.WriteByte(value)
		case objectValueState:
			return processor.consumeValue(value, index)
		case objectCommaState:
			switch value {
			case ',':
				container.key = ""
				container.state = objectKeyState
				return processor.writer.WriteByte(value)
			case '}':
				return processor.closeContainer(value)
			default:
				return ErrInvalidJSON
			}
		}
	}
	switch container.state {
	case arrayValueState:
		if value == ']' {
			return processor.closeContainer(value)
		}
		return processor.consumeValue(value, index)
	case arrayCommaState:
		switch value {
		case ',':
			container.state = arrayValueState
			return processor.writer.WriteByte(value)
		case ']':
			return processor.closeContainer(value)
		default:
			return ErrInvalidJSON
		}
	}
	return ErrInvalidJSON
}

func (processor *jsonProcessor) consumeValue(value byte, parentIndex int) error {
	parent := &processor.stack[parentIndex]
	path := processor.valuePath(*parent)
	if parent.kind == '{' {
		parent.state = objectCommaState
	} else {
		parent.state = arrayCommaState
	}
	if replacementPath(path) && value != '"' {
		return ErrInvalidJSON
	}
	switch value {
	case '"':
		if replacementPath(path) {
			raw, err := processor.readTargetString()
			if err != nil {
				return err
			}
			decoded, err := decodeJSONString(raw)
			if err != nil {
				return ErrInvalidJSON
			}
			processor.assignSelector(path, decoded)
			replacement, replace := processor.rewrite.Replacements[path]
			if replace && (replacement.From == "" || strings.TrimSpace(decoded) == strings.TrimSpace(replacement.From)) {
				encoded, err := json.Marshal(replacement.To)
				if err != nil {
					return err
				}
				_, err = processor.writer.Write(encoded)
				return err
			}
			if err := processor.writer.WriteByte('"'); err != nil {
				return err
			}
			if _, err := processor.writer.Write(raw); err != nil {
				return err
			}
			return processor.writer.WriteByte('"')
		}
		if err := processor.writer.WriteByte(value); err != nil {
			return err
		}
		_, _, err := processor.copyString(0, processor.rewrite.EscapeHTML)
		return err
	case '{':
		if len(processor.stack) >= maxJSONNesting {
			return ErrInvalidJSON
		}
		if err := processor.writer.WriteByte(value); err != nil {
			return err
		}
		override := parent.root && parent.key == "override_settings"
		processor.stack = append(processor.stack, jsonContainer{kind: '{', state: objectKeyState, override: override})
		return nil
	case '[':
		if len(processor.stack) >= maxJSONNesting {
			return ErrInvalidJSON
		}
		if err := processor.writer.WriteByte(value); err != nil {
			return err
		}
		processor.stack = append(processor.stack, jsonContainer{kind: '[', state: arrayValueState})
		return nil
	default:
		token, err := processor.copyPrimitive(value)
		if err != nil {
			return err
		}
		processor.assignPrimitive(path, token)
		return nil
	}
}

func (processor *jsonProcessor) closeContainer(value byte) error {
	if err := processor.writer.WriteByte(value); err != nil {
		return err
	}
	processor.stack = processor.stack[:len(processor.stack)-1]
	if len(processor.stack) == 0 {
		processor.complete = true
	}
	return nil
}

func (processor *jsonProcessor) valuePath(container jsonContainer) string {
	if container.kind != '{' {
		return ""
	}
	if container.root {
		return container.key
	}
	if container.override {
		return "override_settings." + container.key
	}
	return ""
}

func (processor *jsonProcessor) copyString(captureLimit int, escapeHTML bool) ([]byte, bool, error) {
	raw := make([]byte, 0, minInt(captureLimit, 64))
	overflow := false
	for {
		value, err := processor.reader.ReadByte()
		if err != nil {
			return nil, overflow, ErrInvalidJSON
		}
		if value < 0x20 {
			return nil, overflow, ErrInvalidJSON
		}
		if value == '"' {
			if err := processor.writer.WriteByte(value); err != nil {
				return nil, overflow, err
			}
			return raw, overflow, nil
		}
		if captureLimit > 0 && !overflow {
			if len(raw) >= captureLimit {
				overflow = true
				raw = nil
			} else {
				raw = append(raw, value)
			}
		}
		if value == '\\' {
			if err := processor.writer.WriteByte(value); err != nil {
				return nil, overflow, err
			}
			escaped, err := processor.reader.ReadByte()
			if err != nil {
				return nil, overflow, ErrInvalidJSON
			}
			if captureLimit > 0 && !overflow {
				if len(raw) >= captureLimit {
					overflow = true
					raw = nil
				} else {
					raw = append(raw, escaped)
				}
			}
			if !validJSONEscape(escaped) {
				return nil, overflow, ErrInvalidJSON
			}
			if err := processor.writer.WriteByte(escaped); err != nil {
				return nil, overflow, err
			}
			if escaped == 'u' {
				for range 4 {
					digit, readErr := processor.reader.ReadByte()
					if readErr != nil || !jsonHexDigit(digit) {
						return nil, overflow, ErrInvalidJSON
					}
					if captureLimit > 0 && !overflow {
						if len(raw) >= captureLimit {
							overflow = true
							raw = nil
						} else {
							raw = append(raw, digit)
						}
					}
					if err := processor.writer.WriteByte(digit); err != nil {
						return nil, overflow, err
					}
				}
			}
			continue
		}
		if escapeHTML {
			switch value {
			case '<':
				_, err = processor.writer.WriteString(`\u003c`)
			case '>':
				_, err = processor.writer.WriteString(`\u003e`)
			case '&':
				_, err = processor.writer.WriteString(`\u0026`)
			default:
				err = processor.writer.WriteByte(value)
			}
		} else {
			err = processor.writer.WriteByte(value)
		}
		if err != nil {
			return nil, overflow, err
		}
	}
}

func validJSONEscape(value byte) bool {
	switch value {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	default:
		return false
	}
}

func jsonHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func (processor *jsonProcessor) readTargetString() ([]byte, error) {
	raw := make([]byte, 0, 64)
	for {
		value, err := processor.reader.ReadByte()
		if err != nil {
			return nil, ErrInvalidJSON
		}
		if value < 0x20 {
			return nil, ErrInvalidJSON
		}
		if value == '"' {
			return raw, nil
		}
		if len(raw) >= selectorValueLimit {
			return nil, ErrSelectorTooLarge
		}
		raw = append(raw, value)
		if value == '\\' {
			escaped, err := processor.reader.ReadByte()
			if err != nil {
				return nil, ErrInvalidJSON
			}
			if len(raw) >= selectorValueLimit {
				return nil, ErrSelectorTooLarge
			}
			raw = append(raw, escaped)
		}
	}
}

func (processor *jsonProcessor) copyPrimitive(first byte) ([]byte, error) {
	token := []byte{first}
	if err := processor.writer.WriteByte(first); err != nil {
		return nil, err
	}
	for {
		value, err := processor.reader.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if isJSONDelimiter(value) {
			if err := processor.reader.UnreadByte(); err != nil {
				return nil, err
			}
			break
		}
		if len(token) > 128 {
			return nil, ErrInvalidJSON
		}
		token = append(token, value)
		if err := processor.writer.WriteByte(value); err != nil {
			return nil, err
		}
	}
	if !json.Valid(token) {
		return nil, ErrInvalidJSON
	}
	return token, nil
}

func (processor *jsonProcessor) assignSelector(path string, value string) {
	switch path {
	case PathModel:
		processor.fields.Model = strings.TrimSpace(value)
		processor.fields.ModelSet = true
	case PathImageModel:
		processor.fields.ImageModel = strings.TrimSpace(value)
		processor.fields.ImageModelSet = true
	case PathOverrideImageModel:
		processor.fields.OverrideImageModel = strings.TrimSpace(value)
		processor.fields.OverrideModelSet = true
	}
}

func (processor *jsonProcessor) assignPrimitive(path string, token []byte) {
	value, err := strconv.ParseFloat(string(token), 64)
	if err != nil {
		return
	}
	switch path {
	case "width", "image_width", "W":
		processor.fields.Width = int64(value)
		processor.fields.WidthSet = true
	case "height", "image_height", "H":
		processor.fields.Height = int64(value)
		processor.fields.HeightSet = true
	case "n":
		processor.fields.Count = int64(value)
		processor.fields.CountSet = true
	case "batch_size":
		processor.fields.BatchSize = int64(value)
		processor.fields.BatchSizeSet = true
	case "n_iter", "batch_count":
		processor.fields.BatchCount = int64(value)
		processor.fields.BatchCountSet = true
	}
}

func replacementPath(path string) bool {
	return path == PathModel || path == PathImageModel || path == PathOverrideImageModel
}

func decodeJSONString(raw []byte) (string, error) {
	value, err := strconv.Unquote(`"` + string(raw) + `"`)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return value, nil
}

func isJSONWhitespace(value byte) bool {
	return value == ' ' || value == '\n' || value == '\r' || value == '\t'
}

func isJSONDelimiter(value byte) bool {
	return isJSONWhitespace(value) || value == ',' || value == '}' || value == ']'
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
