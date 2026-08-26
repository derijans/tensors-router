package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"tensors-router/internal/transportbody"
)

func (service *Service) adaptBufferedWhisperRequest(request *http.Request, body []byte) ([]byte, error) {
	if request.URL.Path == "/api/extra/transcribe" && transportRequestIsJSON(request) {
		return adaptKoboldTranscriptionRequest(request, body)
	}
	mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") || params["boundary"] == "" {
		return nil, fmt.Errorf("transcription request must be multipart/form-data")
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var output bytes.Buffer
	writer := multipart.NewWriter(&output)
	format := "json"
	hasFile := false
	writtenFormat := false
	writtenTranslate := false
	for {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			return nil, partErr
		}
		name := part.FormName()
		if name == "model" {
			_ = part.Close()
			continue
		}
		if name == "response_format" {
			value, readErr := io.ReadAll(io.LimitReader(part, 65))
			_ = part.Close()
			if readErr != nil {
				return nil, readErr
			}
			format = strings.TrimSpace(string(value))
			if !validWhisperResponseFormat(format) {
				return nil, fmt.Errorf("unsupported transcription response format %q", format)
			}
			if err := writer.WriteField("response_format", "verbose_json"); err != nil {
				return nil, err
			}
			writtenFormat = true
			continue
		}
		if name == "translate" {
			_ = part.Close()
			if err := writer.WriteField("translate", strconv.FormatBool(request.URL.Path == "/v1/audio/translations")); err != nil {
				return nil, err
			}
			writtenTranslate = true
			continue
		}
		if name == "file" {
			if err := service.writeWhisperFilePart(request, writer, part); err != nil {
				_ = part.Close()
				return nil, err
			}
			hasFile = true
			_ = part.Close()
			continue
		}
		target, createErr := writer.CreatePart(part.Header)
		if createErr != nil {
			_ = part.Close()
			return nil, createErr
		}
		if _, err := transportbody.Copy(target, part); err != nil {
			_ = part.Close()
			return nil, err
		}
		_ = part.Close()
	}
	if !hasFile {
		return nil, fmt.Errorf("transcription file is required")
	}
	if !writtenFormat {
		if err := writer.WriteField("response_format", "verbose_json"); err != nil {
			return nil, err
		}
	}
	if !writtenTranslate {
		if err := writer.WriteField("translate", strconv.FormatBool(request.URL.Path == "/v1/audio/translations")); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Tensors-Whisper-Response-Format", format)
	request.ContentLength = int64(output.Len())
	return output.Bytes(), nil
}

// writeWhisperFilePart copies a native WAV file part unchanged. A non-WAV
// part is converted with ffmpeg when available; whisper.cpp's transcription
// endpoint only ever accepts WAV, and ffmpeg conversion is only available on
// this buffered path — the streaming (large-body) whisper path still
// requires native WAV input.
func (service *Service) writeWhisperFilePart(request *http.Request, writer *multipart.Writer, part *multipart.Part) error {
	header := make([]byte, 12)
	read, readErr := io.ReadFull(part, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return readErr
	}
	if isWAVHeader(header[:read]) {
		target, err := writer.CreatePart(part.Header)
		if err != nil {
			return err
		}
		if _, err := target.Write(header[:read]); err != nil {
			return err
		}
		_, err = transportbody.Copy(target, part)
		return err
	}
	if !service.ffmpeg.Available() {
		return fmt.Errorf("only native WAV transcription input is supported, and ffmpeg is not available on this router to convert other formats")
	}
	rest, err := io.ReadAll(part)
	if err != nil {
		return err
	}
	var wav bytes.Buffer
	source := io.MultiReader(bytes.NewReader(header[:read]), bytes.NewReader(rest))
	if err := service.ffmpeg.ConvertToWAV(request.Context(), source, &wav); err != nil {
		return fmt.Errorf("ffmpeg could not convert transcription input to WAV: %w", err)
	}
	target, err := writer.CreatePart(wavPartHeader(part.Header))
	if err != nil {
		return err
	}
	_, err = target.Write(wav.Bytes())
	return err
}

func wavPartHeader(original textproto.MIMEHeader) textproto.MIMEHeader {
	cloned := make(textproto.MIMEHeader, len(original))
	for key, values := range original {
		cloned[key] = append([]string{}, values...)
	}
	cloned.Set("Content-Type", "audio/wav")
	return cloned
}

func adaptKoboldTranscriptionRequest(request *http.Request, body []byte) ([]byte, error) {
	var input struct {
		AudioData         string `json:"audio_data"`
		Prompt            string `json:"prompt"`
		LangCode          string `json:"langcode"`
		Language          string `json:"language"`
		SuppressNonSpeech bool   `json:"suppress_non_speech"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, fmt.Errorf("invalid transcription JSON: %w", err)
	}
	encoded := input.AudioData
	if _, value, found := strings.Cut(encoded, ","); found {
		encoded = value
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	header := make([]byte, 12)
	read, err := io.ReadFull(decoder, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("audio_data is not valid base64: %w", err)
	}
	if !isWAVHeader(header[:read]) {
		return nil, fmt.Errorf("only native WAV transcription input is supported")
	}
	var output bytes.Buffer
	writer := multipart.NewWriter(&output)
	file, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(header[:read]); err != nil {
		return nil, err
	}
	if _, err := transportbody.Copy(file, decoder); err != nil {
		return nil, fmt.Errorf("audio_data is not valid base64: %w", err)
	}
	if input.Prompt != "" {
		_ = writer.WriteField("prompt", input.Prompt)
	}
	language := input.LangCode
	if language == "" {
		language = input.Language
	}
	if language != "" {
		_ = writer.WriteField("language", language)
	}
	if input.SuppressNonSpeech {
		_ = writer.WriteField("suppress_nst", "true")
	}
	_ = writer.WriteField("response_format", "verbose_json")
	if err := writer.Close(); err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Tensors-Whisper-Response-Format", "json")
	request.ContentLength = int64(output.Len())
	return output.Bytes(), nil
}

func validWhisperResponseFormat(format string) bool {
	switch format {
	case "json", "verbose_json", "text", "srt", "vtt":
		return true
	default:
		return false
	}
}

func isWAVHeader(header []byte) bool {
	return len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WAVE"
}

type whisperVerboseResponse struct {
	Task     string           `json:"task,omitempty"`
	Language string           `json:"language,omitempty"`
	Duration float64          `json:"duration,omitempty"`
	Text     string           `json:"text"`
	Segments []whisperSegment `json:"segments,omitempty"`
}

type whisperSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func adaptWhisperResponse(response *http.Response, format string) (*http.Response, error) {
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, transportbody.TransformationWorkingSet+1))
	closeErr := response.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(body)) > transportbody.TransformationWorkingSet {
		return nil, transportbody.ErrResponseTooLarge
	}
	var verbose whisperVerboseResponse
	if err := json.Unmarshal(body, &verbose); err != nil {
		return nil, fmt.Errorf("whisper-server returned invalid verbose JSON: %w", err)
	}
	response.Header.Set("X-Tensors-Audio-Language", verbose.Language)
	response.Header.Set("X-Tensors-Audio-Task", verbose.Task)
	response.Header.Set("X-Tensors-Audio-Duration", strconv.FormatFloat(verbose.Duration, 'f', -1, 64))
	var rendered []byte
	contentType := "application/json"
	switch format {
	case "verbose_json":
		rendered = body
	case "text":
		rendered = []byte(verbose.Text)
		contentType = "text/plain; charset=utf-8"
	case "srt":
		rendered = []byte(renderWhisperSubtitles(verbose.Segments, false))
		contentType = "application/x-subrip; charset=utf-8"
	case "vtt":
		rendered = []byte("WEBVTT\n\n" + renderWhisperSubtitles(verbose.Segments, true))
		contentType = "text/vtt; charset=utf-8"
	default:
		rendered, err = json.Marshal(map[string]string{"text": verbose.Text})
		if err != nil {
			return nil, err
		}
	}
	response.Body = io.NopCloser(bytes.NewReader(rendered))
	response.ContentLength = int64(len(rendered))
	response.Header.Set("Content-Type", contentType)
	response.Header.Set("Content-Length", strconv.Itoa(len(rendered)))
	return response, nil
}

func renderWhisperSubtitles(segments []whisperSegment, vtt bool) string {
	var output strings.Builder
	for index, segment := range segments {
		if !vtt {
			output.WriteString(strconv.Itoa(index + 1))
			output.WriteByte('\n')
		}
		output.WriteString(whisperTimestamp(segment.Start, vtt))
		output.WriteString(" --> ")
		output.WriteString(whisperTimestamp(segment.End, vtt))
		output.WriteByte('\n')
		output.WriteString(strings.TrimSpace(segment.Text))
		output.WriteString("\n\n")
	}
	return output.String()
}

func whisperTimestamp(seconds float64, vtt bool) string {
	if seconds < 0 {
		seconds = 0
	}
	duration := time.Duration(seconds * float64(time.Second))
	hours := duration / time.Hour
	duration -= hours * time.Hour
	minutes := duration / time.Minute
	duration -= minutes * time.Minute
	wholeSeconds := duration / time.Second
	milliseconds := (duration - wholeSeconds*time.Second) / time.Millisecond
	separator := ","
	if vtt {
		separator = "."
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", hours, minutes, wholeSeconds, separator, milliseconds)
}
