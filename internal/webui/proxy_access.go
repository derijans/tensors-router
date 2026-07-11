package webui

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const backendSessionCookie = "tensors_router_backend"

type proxyAccessGrant struct {
	kind      string
	expiresAt time.Time
}

type proxyAccessManager struct {
	mu       sync.Mutex
	tickets  map[string]proxyAccessGrant
	sessions map[string]proxyAccessGrant
	now      func() time.Time
}

func newProxyAccessManager() *proxyAccessManager {
	return &proxyAccessManager{
		tickets:  map[string]proxyAccessGrant{},
		sessions: map[string]proxyAccessGrant{},
		now:      time.Now,
	}
}

func (manager *proxyAccessManager) Issue(kind string) string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.removeExpiredLocked()
	ticket := randomAccessToken()
	manager.tickets[ticket] = proxyAccessGrant{kind: kind, expiresAt: manager.now().Add(time.Minute)}
	return ticket
}

func (manager *proxyAccessManager) Exchange(w http.ResponseWriter, ticket string, kind string) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.removeExpiredLocked()
	grant, ok := manager.tickets[ticket]
	delete(manager.tickets, ticket)
	if !ok || grant.kind != kind || !manager.now().Before(grant.expiresAt) {
		return false
	}
	session := randomAccessToken()
	expiresAt := manager.now().Add(15 * time.Minute)
	manager.sessions[session] = proxyAccessGrant{kind: kind, expiresAt: expiresAt}
	http.SetCookie(w, &http.Cookie{
		Name:     backendSessionCookie,
		Value:    session,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   900,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return true
}

func (manager *proxyAccessManager) Authorized(r *http.Request, kind string) bool {
	cookie, err := r.Cookie(backendSessionCookie)
	if err != nil {
		return false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.removeExpiredLocked()
	grant, ok := manager.sessions[cookie.Value]
	return ok && grant.kind == kind && manager.now().Before(grant.expiresAt)
}

func (manager *proxyAccessManager) removeExpiredLocked() {
	now := manager.now()
	for token, grant := range manager.tickets {
		if !now.Before(grant.expiresAt) {
			delete(manager.tickets, token)
		}
	}
	for token, grant := range manager.sessions {
		if !now.Before(grant.expiresAt) {
			delete(manager.sessions, token)
		}
	}
}

func randomAccessToken() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
