package proxy

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"tensors-router/internal/transportbody"
)

const whisperResponseFormatReadLimit = 65

type whisperFormRewrite struct {
	service          *Service
	request          *http.Request
	writer           *multipart.Writer
	format           string
	hasFile          bool
	writtenFormat    bool
	writtenTranslate bool
}

func (service *Service) adaptBufferedWhisperRequest(request *http.Request, body []byte) ([]byte, error) {
	if request.URL.Path == pathKoboldTranscribe && transportRequestIsJSON(request) {
		return adaptKoboldTranscriptionRequest(request, body)
	}
	mediaType, params, err := mime.ParseMediaType(request.Header.Get(headerContentType))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") || params["boundary"] == "" {
		return nil, fmt.Errorf("transcription request must be multipart/form-data")
	}
	var output bytes.Buffer
	rewrite := whisperFormRewrite{service: service, request: request, writer: multipart.NewWriter(&output), format: "json"}
	if err := rewrite.copyParts(multipart.NewReader(bytes.NewReader(body), params["boundary"])); err != nil {
		return nil, err
	}
	if err := rewrite.finish(); err != nil {
		return nil, err
	}
	request.Header.Set(headerContentType, rewrite.writer.FormDataContentType())
	request.Header.Set("X-Tensors-Whisper-Response-Format", rewrite.format)
	request.ContentLength = int64(output.Len())
	return output.Bytes(), nil
}

func (rewrite *whisperFormRewrite) copyParts(reader *multipart.Reader) error {
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		err = rewrite.copyPart(part)
		_ = part.Close()
		if err != nil {
			return err
		}
	}
}

func (rewrite *whisperFormRewrite) copyPart(part *multipart.Part) error {
	switch part.FormName() {
	case "model":
		return nil
	case "response_format":
		return rewrite.replaceResponseFormat(part)
	case "translate":
		return rewrite.writeTranslate()
	case "file":
		if err := rewrite.service.writeWhisperFilePart(rewrite.request, rewrite.writer, part); err != nil {
			return err
		}
		rewrite.hasFile = true
		return nil
	default:
		return copyMultipartPart(rewrite.writer, part)
	}
}

func (rewrite *whisperFormRewrite) replaceResponseFormat(part *multipart.Part) error {
	value, err := io.ReadAll(io.LimitReader(part, whisperResponseFormatReadLimit))
	if err != nil {
		return err
	}
	rewrite.format = strings.TrimSpace(string(value))
	if !validWhisperResponseFormat(rewrite.format) {
		return fmt.Errorf("unsupported transcription response format %q", rewrite.format)
	}
	rewrite.writtenFormat = true
	return rewrite.writer.WriteField("response_format", "verbose_json")
}

func (rewrite *whisperFormRewrite) writeTranslate() error {
	rewrite.writtenTranslate = true
	return rewrite.writer.WriteField("translate", strconv.FormatBool(rewrite.request.URL.Path == pathAudioTranslations))
}

func (rewrite *whisperFormRewrite) finish() error {
	if !rewrite.hasFile {
		return fmt.Errorf("transcription file is required")
	}
	if !rewrite.writtenFormat {
		if err := rewrite.writer.WriteField("response_format", "verbose_json"); err != nil {
			return err
		}
	}
	if !rewrite.writtenTranslate {
		if err := rewrite.writeTranslate(); err != nil {
			return err
		}
	}
	return rewrite.writer.Close()
}

func copyMultipartPart(writer *multipart.Writer, part *multipart.Part) error {
	target, err := writer.CreatePart(part.Header)
	if err != nil {
		return err
	}
	_, err = transportbody.Copy(target, part)
	return err
}
