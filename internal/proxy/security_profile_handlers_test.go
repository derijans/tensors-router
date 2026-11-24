package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/credentialstore"
)

type profileFileSpy struct {
	blocker error
	writes  int
}

func (spy *profileFileSpy) control() SecurityProfileControl {
	return SecurityProfileControl{
		Running:         "trusted_lan",
		SecureBlocker:   func() error { return spy.blocker },
		WriteSecureFile: func() error { spy.writes++; return nil },
	}
}

func newProfileTestRoutes(t *testing.T, spy *profileFileSpy) *routeTable {
	t.Helper()
	ctx := context.Background()
	store, err := credentialstore.Open(ctx, credentialstore.Config{Path: filepath.Join(t.TempDir(), credentialstore.Filename)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	live, err := credentialstore.NewLive(ctx, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	admin := newSecurityAdmin(SecurityConfig{Credentials: live, Profile: spy.control()})
	return newRouteTable(func(http.ResponseWriter, *http.Request) bool { return true }, admin.routes())
}

func switchProfile(routes *routeTable, bearer string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, securityProfilePath, strings.NewReader("{}"))
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	routes.serve(recorder, request)
	return recorder
}

func profileStatus(t *testing.T, routes *routeTable) securityStatusResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	routes.serve(recorder, httptest.NewRequest(http.MethodGet, securityStatusPath, nil))
	var status securityStatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func createAdminToken(t *testing.T, routes *routeTable) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	routes.serve(recorder, httptest.NewRequest(http.MethodPost, securityTokensPath, strings.NewReader(`{"name":"ops","role":"admin"}`)))
	var response securityTokenCreatedResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Secret == "" {
		t.Fatalf("admin token not created: %s", recorder.Body)
	}
	return response.Secret
}

func TestSecureProfileSwitchRefusesWhileTheRouterCouldNotStartSecure(t *testing.T) {
	spy := &profileFileSpy{blocker: errors.New("an inference token is required")}
	routes := newProfileTestRoutes(t, spy)
	if status := profileStatus(t, routes); status.SecureBlocker != "an inference token is required" {
		t.Fatalf("status did not report the blocker: %#v", status)
	}
	if recorder := switchProfile(routes, ""); recorder.Code != http.StatusConflict || spy.writes != 0 {
		t.Fatalf("switch status %d writes %d", recorder.Code, spy.writes)
	}
}

func TestSecureProfileSwitchRequiresAdminTokenOnceAdminTokensExist(t *testing.T) {
	spy := &profileFileSpy{}
	routes := newProfileTestRoutes(t, spy)
	secret := createAdminToken(t, routes)
	if recorder := switchProfile(routes, ""); recorder.Code != http.StatusConflict || spy.writes != 0 {
		t.Fatalf("switch without admin token: status %d writes %d", recorder.Code, spy.writes)
	}
	if recorder := switchProfile(routes, secret); recorder.Code != http.StatusOK || spy.writes != 1 {
		t.Fatalf("switch with admin token: status %d writes %d: %s", recorder.Code, spy.writes, recorder.Body)
	}
	status := profileStatus(t, routes)
	if status.Profile != "trusted_lan" || !status.ProfileRestartRequired || status.SecureBlocker != "" {
		t.Fatalf("status after switch %#v", status)
	}
}
