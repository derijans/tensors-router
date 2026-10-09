package transportbody

const unicodeEscapeDigits = 4

type stringCapture struct {
	limit    int
	raw      []byte
	overflow bool
}

func (capture *stringCapture) add(value byte) {
	if capture.limit <= 0 || capture.overflow {
		return
	}
	if len(capture.raw) >= capture.limit {
		capture.overflow = true
		capture.raw = nil
		return
	}
	capture.raw = append(capture.raw, value)
}

func (processor *jsonProcessor) copyString(captureLimit int, escapeHTML bool) ([]byte, bool, error) {
	capture := stringCapture{limit: captureLimit, raw: make([]byte, 0, min(captureLimit, 64))}
	for {
		value, err := processor.reader.ReadByte()
		if err != nil || value < 0x20 {
			return nil, capture.overflow, ErrInvalidJSON
		}
		if value == '"' {
			if err := processor.writer.WriteByte(value); err != nil {
				return nil, capture.overflow, err
			}
			return capture.raw, capture.overflow, nil
		}
		capture.add(value)
		if value == '\\' {
			err = processor.copyStringEscape(&capture)
		} else {
			err = processor.writeStringByte(value, escapeHTML)
		}
		if err != nil {
			return nil, capture.overflow, err
		}
	}
}

func (processor *jsonProcessor) copyStringEscape(capture *stringCapture) error {
	if err := processor.writer.WriteByte('\\'); err != nil {
		return err
	}
	escaped, err := processor.reader.ReadByte()
	if err != nil {
		return ErrInvalidJSON
	}
	capture.add(escaped)
	if !validJSONEscape(escaped) {
		return ErrInvalidJSON
	}
	if err := processor.writer.WriteByte(escaped); err != nil {
		return err
	}
	if escaped == 'u' {
		return processor.copyUnicodeEscapeDigits(capture)
	}
	return nil
}

func (processor *jsonProcessor) copyUnicodeEscapeDigits(capture *stringCapture) error {
	for range unicodeEscapeDigits {
		digit, err := processor.reader.ReadByte()
		if err != nil || !jsonHexDigit(digit) {
			return ErrInvalidJSON
		}
		capture.add(digit)
		if err := processor.writer.WriteByte(digit); err != nil {
			return err
		}
	}
	return nil
}
