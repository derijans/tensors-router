package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/transportbody"
)

func chatTemplateProfileForRequest(path string, profile catalog.ChatTemplateProfile) *transportbody.ChatTemplateKwargsRewrite {
	if path != "/v1/chat/completions" || !profile.HasConfiguredKwargs() {
		return nil
	}
	return &transportbody.ChatTemplateKwargsRewrite{
		Configured: profile.ConfiguredKwargs(),
		ConfigWins: profile.Precedence() == catalog.JinjaKwargsPrecedenceConfig,
	}
}

func requestJSONRewrite(path string, localID string, readiness backendReadiness, profile catalog.ChatTemplateProfile, rewriteSelectors bool) transportbody.JSONRewrite {
	rewrite := transportbody.JSONRewrite{
		ChatTemplateKwargs: chatTemplateProfileForRequest(path, profile),
	}
	if !rewriteSelectors || strings.TrimSpace(localID) == "" {
		return rewrite
	}
	rewrite.Replacements = map[string]transportbody.StringReplacement{
		transportbody.PathModel: {To: localID},
	}
	if readiness == readinessImage {
		rewrite.Replacements[transportbody.PathImageModel] = transportbody.StringReplacement{To: localID}
		rewrite.Replacements[transportbody.PathOverrideImageModel] = transportbody.StringReplacement{To: localID}
	}
	return rewrite
}

func transformBufferedTransportRequestBody(r *http.Request, body []byte, localID string, readiness backendReadiness, profile catalog.ChatTemplateProfile, rewriteSelectors bool) ([]byte, error) {
	if len(body) == 0 || !requestBodyLooksJSON(body, r) {
		return body, nil
	}
	rewrite := requestJSONRewrite(r.URL.Path, localID, readiness, profile, rewriteSelectors)
	if len(rewrite.Replacements) == 0 && rewrite.ChatTemplateKwargs == nil {
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
