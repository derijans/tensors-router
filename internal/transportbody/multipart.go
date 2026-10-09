package transportbody

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
)

type MultipartRewrite struct {
	Fields     map[string]StringReplacement
	DropFields map[string]bool
}

func InspectMultipartField(body Body, boundary string, fieldName string) (string, bool, error) {
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
		if part.FormName() != fieldName {
			_ = part.Close()
			continue
		}
		value, err := readSelectorPart(part)
		_ = part.Close()
		return value, true, err
	}
}

func InspectMultipartFileHeader(body Body, boundary string, fieldName string, size int) ([]byte, bool, error) {
	if body == nil || !body.Replayable() {
		return nil, false, ErrSelectorRequired
	}
	attempt, err := body.OpenAttempt()
	if err != nil {
		return nil, false, err
	}
	defer attempt.Close()
	reader := multipart.NewReader(attempt, boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if part.FormName() != fieldName {
			_ = part.Close()
			continue
		}
		header := make([]byte, size)
		read, readErr := io.ReadFull(part, header)
		_ = part.Close()
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return nil, false, readErr
		}
		return header[:read], true, nil
	}
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

type multipartRewriter struct {
	writer        *multipart.Writer
	rewrite       MultipartRewrite
	writtenFields map[string]bool
}

func rewriteMultipart(source io.Reader, destination io.Writer, oldBoundary string, newBoundary string, rewrite MultipartRewrite) error {
	reader := multipart.NewReader(source, oldBoundary)
	rewriter := multipartRewriter{writer: multipart.NewWriter(destination), rewrite: rewrite, writtenFields: make(map[string]bool)}
	if err := rewriter.writer.SetBoundary(newBoundary); err != nil {
		return err
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return rewriter.finish()
		}
		if err != nil {
			return err
		}
		if err := rewriter.copyPart(part); err != nil {
			return err
		}
	}
}

func (rewriter *multipartRewriter) copyPart(part *multipart.Part) error {
	name := part.FormName()
	if rewriter.rewrite.DropFields[name] {
		_ = part.Close()
		return nil
	}
	rewriter.writtenFields[name] = true
	target, err := rewriter.writer.CreatePart(cloneMIMEHeader(part.Header))
	if err != nil {
		_ = part.Close()
		return err
	}
	if replacement, selected := rewriter.rewrite.Fields[name]; selected {
		return writeReplacedSelector(target, part, replacement)
	}
	if _, err := Copy(target, part); err != nil {
		_ = part.Close()
		return err
	}
	return part.Close()
}

func writeReplacedSelector(target io.Writer, part *multipart.Part, replacement StringReplacement) error {
	value, err := readSelectorPart(part)
	_ = part.Close()
	if err != nil {
		return err
	}
	if replacement.From == "" || strings.TrimSpace(value) == strings.TrimSpace(replacement.From) {
		value = replacement.To
	}
	_, err = io.WriteString(target, value)
	return err
}

func (rewriter *multipartRewriter) finish() error {
	for name, replacement := range rewriter.rewrite.Fields {
		if rewriter.writtenFields[name] || rewriter.rewrite.DropFields[name] {
			continue
		}
		target, err := rewriter.writer.CreateFormField(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(target, replacement.To); err != nil {
			return err
		}
	}
	return rewriter.writer.Close()
}
