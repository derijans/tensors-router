package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/transportbody"
)

var chatTemplatePaths = map[string]bool{
	"/v1/chat/completions": true,
	pathOllamaChat:         true,
	pathOllamaGenerate:     true,
}

func chatTemplateProfileForRequest(path string, profile catalog.ChatTemplateProfile) *transportbody.ChatTemplateKwargsRewrite {
	if !chatTemplatePaths[path] || !profile.HasConfiguredKwargs() {
		return nil
	}
	return &transportbody.ChatTemplateKwargsRewrite{
		Configured: profile.ConfiguredKwargs(),
		ConfigWins: profile.Precedence() == catalog.JinjaKwargsPrecedenceConfig,
	}
}

var generationDefaultsTopLevelPaths = map[string]bool{
	"/v1/chat/completions":       true,
	"/v1/completions":            true,
	"/v1/responses":              true,
	"/v1/messages":               true,
	"/api/v1/generate":           true,
	"/api/extra/generate/stream": true,
}

func generationDefaultsForRequest(path string, profile catalog.ChatTemplateProfile) *transportbody.GenerationDefaultsRewrite {
	defaults := profile.GenerationDefaults()
	backendReadsTopLevel := generationDefaultsTopLevelPaths[path] || (path == pathOllamaChat && defaults.OllamaChatReadsTopLevel)
	ollamaRequest := path == pathOllamaGenerate || path == pathOllamaChat
	switch {
	case backendReadsTopLevel && len(defaults.TopLevel) > 0:
		return &transportbody.GenerationDefaultsRewrite{Defaults: jsonFields(defaults.TopLevel), Overwrite: defaults.Overwrite}
	case ollamaRequest && !backendReadsTopLevel && len(defaults.OllamaOptions) > 0:
		return &transportbody.GenerationDefaultsRewrite{Defaults: jsonFields(defaults.OllamaOptions), Overwrite: defaults.Overwrite, IntoOllamaOptions: true}
	default:
		return nil
	}
}

func jsonFields(defaults []catalog.GenerationDefault) []transportbody.JSONField {
	fields := make([]transportbody.JSONField, 0, len(defaults))
	for _, item := range defaults {
		fields = append(fields, transportbody.JSONField{Key: item.Key, Value: item.Value})
	}
	return fields
}

func requestJSONRewrite(path string, localID string, readiness backendReadiness, profile catalog.ChatTemplateProfile, rewriteSelectors bool, insertModel bool) transportbody.JSONRewrite {
	rewrite := transportbody.JSONRewrite{
		ChatTemplateKwargs: chatTemplateProfileForRequest(path, profile),
		GenerationDefaults: generationDefaultsForRequest(path, profile),
	}
	if !rewriteSelectors || strings.TrimSpace(localID) == "" {
		return rewrite
	}
	rewrite.Replacements = map[string]transportbody.StringReplacement{
		transportbody.PathModel: {To: localID},
	}
	if insertModel {
		rewrite.Insertions = map[string]string{transportbody.PathModel: localID}
	}
	if readiness == readinessImage {
		rewrite.Replacements[transportbody.PathImageModel] = transportbody.StringReplacement{To: localID}
		rewrite.Replacements[transportbody.PathOverrideImageModel] = transportbody.StringReplacement{To: localID}
	}
	return rewrite
}

func transformBufferedTransportRequestBody(r *http.Request, body []byte, localID string, readiness backendReadiness, profile catalog.ChatTemplateProfile, rewriteSelectors bool, insertModel bool) ([]byte, error) {
	if len(body) == 0 || !requestBodyLooksJSON(body, r) {
		return body, nil
	}
	rewrite := requestJSONRewrite(r.URL.Path, localID, readiness, profile, rewriteSelectors, insertModel)
	if len(rewrite.Replacements) == 0 && rewrite.ChatTemplateKwargs == nil && rewrite.GenerationDefaults == nil {
		return body, nil
	}
	source := transportbody.NewJSONTransformReadCloser(io.NopCloser(bytes.NewReader(body)), rewrite)
	defer source.Close()
	return io.ReadAll(source)
}

func (service *Service) localChatTemplateProfile(configFilename string, remote bool) catalog.ChatTemplateProfile {
	if remote {
		return catalog.ChatTemplateProfile{}
	}
	return service.chatTemplateProfileForConfig(configFilename)
}
