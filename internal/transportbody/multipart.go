package transportbody

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
)

type MultipartRewrite struct {
	Fields map[string]StringReplacement
}

func InspectMultipartModel(body Body, boundary string) (string, bool, error) {
	if body == nil || !body.Replayable() {
		return "", false, ErrSelectorRequired
	}
	attempt, err := body.OpenAttempt()
	if err != nil {
		return "", false, err
	}
	defer attempt.Close()
	reader := multipart.NewReader(attempt, boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if part.FormName() != "model" {
			_ = part.Close()
			continue
		}
		value, err := readSelectorPart(part)
		_ = part.Close()
		if err != nil {
			return "", false, err
		}
		value = strings.TrimSpace(value)
		return value, value != "", nil
	}
}

func TransformMultipart(body Body, oldBoundary string, rewrite MultipartRewrite) (Body, string, error) {
	boundaryWriter := multipart.NewWriter(io.Discard)
	newBoundary := boundaryWriter.Boundary()
	if err := boundaryWriter.Close(); err != nil {
		return nil, "", err
	}
	transformed := Transform(body, 0, false, func(source io.ReadCloser) (io.ReadCloser, error) {
		return newLazyTransformReadCloser(source, func(reader io.Reader, writer io.Writer) error {
			return rewriteMultipart(reader, writer, oldBoundary, newBoundary, rewrite)
		}), nil
	})
	return transformed, newBoundary, nil
}

func rewriteMultipart(source io.Reader, destination io.Writer, oldBoundary string, newBoundary string, rewrite MultipartRewrite) error {
	reader := multipart.NewReader(source, oldBoundary)
	writer := multipart.NewWriter(destination)
	if err := writer.SetBoundary(newBoundary); err != nil {
		return err
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return writer.Close()
		}
		if err != nil {
			return err
		}
		target, err := writer.CreatePart(cloneMIMEHeader(part.Header))
		if err != nil {
			_ = part.Close()
			return err
		}
		replacement, selected := rewrite.Fields[part.FormName()]
		if selected {
			value, readErr := readSelectorPart(part)
			_ = part.Close()
			if readErr != nil {
				return readErr
			}
			if replacement.From == "" || strings.TrimSpace(value) == strings.TrimSpace(replacement.From) {
				value = replacement.To
			}
			if _, err := io.WriteString(target, value); err != nil {
				return err
			}
			continue
		}
		if _, err := Copy(target, part); err != nil {
			_ = part.Close()
			return err
		}
		if err := part.Close(); err != nil {
			return err
		}
	}
}

func readSelectorPart(reader io.Reader) (string, error) {
	content, err := io.ReadAll(io.LimitReader(reader, selectorValueLimit+1))
	if err != nil {
		return "", err
	}
	if len(content) > selectorValueLimit {
		return "", ErrSelectorTooLarge
	}
	return string(content), nil
}

func cloneMIMEHeader(source textproto.MIMEHeader) textproto.MIMEHeader {
	cloned := make(textproto.MIMEHeader, len(source))
	for key, values := range source {
		cloned[key] = append([]string{}, values...)
	}
	return cloned
}

func MultipartContentType(mediaType string, boundary string) string {
	return fmt.Sprintf("%s; boundary=%q", mediaType, boundary)
}
