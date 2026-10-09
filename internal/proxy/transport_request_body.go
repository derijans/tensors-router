package proxy

import (
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/transportbody"
)

const wavHeaderLength = 12

func transformTransportRequestBody(r *http.Request, body transportbody.Body, route transportRoute, profile catalog.ChatTemplateProfile) (transportbody.Body, error) {
	if strings.TrimSpace(route.localID) == "" && chatTemplateProfileForRequest(r.URL.Path, profile) == nil {
		return body, nil
	}
	if transportRequestIsJSON(r) {
		return transportbody.TransformJSON(body, requestJSONRewrite(r.URL.Path, route.localID, route.readiness, profile, true, route.insertModel)), nil
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get(headerContentType))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		return body, nil
	}
	return transformTransportMultipartBody(r, body, route, mediaType, params["boundary"])
}

func transformTransportMultipartBody(r *http.Request, body transportbody.Body, route transportRoute, mediaType string, boundary string) (transportbody.Body, error) {
	if boundary == "" {
		return nil, fmt.Errorf("multipart boundary is required")
	}
	rewrite := transportbody.MultipartRewrite{
		Fields:     map[string]transportbody.StringReplacement{"model": {To: route.localID}},
		DropFields: map[string]bool{},
	}
	if route.backendMode == BackendModeLlamaSDCPP && route.readiness == readinessTranscription {
		if err := rewriteMultipartForWhisper(r, body, boundary, rewrite); err != nil {
			return nil, err
		}
	}
	transformed, newBoundary, err := transportbody.TransformMultipart(body, boundary, rewrite)
	if err != nil {
		return nil, err
	}
	r.Header.Set(headerContentType, transportbody.MultipartContentType(mediaType, newBoundary))
	return transformed, nil
}

func rewriteMultipartForWhisper(r *http.Request, body transportbody.Body, boundary string, rewrite transportbody.MultipartRewrite) error {
	if err := requireNativeWAVUpload(body, boundary); err != nil {
		return err
	}
	format, err := streamedWhisperResponseFormat(body, boundary)
	if err != nil {
		return err
	}
	r.Header.Set("X-Tensors-Whisper-Response-Format", format)
	rewrite.Fields["response_format"] = transportbody.StringReplacement{To: "verbose_json"}
	rewrite.Fields["translate"] = transportbody.StringReplacement{To: strconv.FormatBool(r.URL.Path == pathAudioTranslations)}
	rewrite.DropFields["model"] = true
	return nil
}

func requireNativeWAVUpload(body transportbody.Body, boundary string) error {
	header, hasFile, err := transportbody.InspectMultipartFileHeader(body, boundary, "file", wavHeaderLength)
	if err != nil {
		return err
	}
	if !hasFile {
		return fmt.Errorf("transcription file is required")
	}
	if !isWAVHeader(header) {
		return fmt.Errorf("only native WAV transcription input is supported for uploads this large; ffmpeg conversion is only available on the buffered (smaller) transcription path")
	}
	return nil
}

func streamedWhisperResponseFormat(body transportbody.Body, boundary string) (string, error) {
	format, present, err := transportbody.InspectMultipartField(body, boundary, "response_format")
	if err != nil && err != transportbody.ErrSelectorRequired {
		return "", err
	}
	if !present || strings.TrimSpace(format) == "" {
		return "json", nil
	}
	if !validWhisperResponseFormat(format) {
		return "", fmt.Errorf("unsupported transcription response format %q", format)
	}
	return format, nil
}
