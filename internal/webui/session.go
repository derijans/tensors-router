package webui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionCookieName = "tensor_router_webui_session"

type SessionManager struct {
	mu       sync.Mutex
	token    string
	sessions map[string]Session
}

type Session struct {
	ID      string
	CSRF    string
	Expires time.Time
}

func NewSessionManager(token string) *SessionManager {
	return &SessionManager{
		token:    token,
		sessions: map[string]Session{},
	}
}

func (manager *SessionManager) Login(w http.ResponseWriter, token string) (Session, bool) {
	if subtle.ConstantTimeCompare([]byte(token), []byte(manager.token)) != 1 {
		return Session{}, false
	}
	session := Session{
		ID:      randomHex(32),
		CSRF:    randomHex(32),
		Expires: time.Now().Add(24 * time.Hour),
	}
	manager.mu.Lock()
	manager.sessions[session.ID] = session
	manager.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.Expires,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return session, true
}

func (manager *SessionManager) Logout(w http.ResponseWriter, r *http.Request) {
	if session, ok := manager.Session(r); ok {
		manager.mu.Lock()
		delete(manager.sessions, session.ID)
		manager.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (manager *SessionManager) Session(r *http.Request) (Session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return Session{}, false
	}
	id := strings.TrimSpace(cookie.Value)
	if id == "" {
		return Session{}, false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	session, ok := manager.sessions[id]
	if !ok || time.Now().After(session.Expires) {
		delete(manager.sessions, id)
		return Session{}, false
	}
	return session, true
}

func (manager *SessionManager) Authorized(r *http.Request) bool {
	_, ok := manager.Session(r)
	return ok
}

func (manager *SessionManager) ValidCSRF(r *http.Request) bool {
	session, ok := manager.Session(r)
	if !ok {
		return false
	}
	token := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(session.CSRF)) == 1
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
