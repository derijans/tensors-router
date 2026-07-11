package webui

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyAccessTicketIsSingleUseAndKindBound(t *testing.T) {
	manager := newProxyAccessManager()
	ticket := manager.Issue("llama")
	wrongKind := httptest.NewRecorder()
	if manager.Exchange(wrongKind, ticket, "sdcpp") {
		t.Fatal("ticket authorized the wrong backend kind")
	}
	replay := httptest.NewRecorder()
	if manager.Exchange(replay, ticket, "llama") {
		t.Fatal("failed ticket was reusable")
	}

	ticket = manager.Issue("llama")
	exchange := httptest.NewRecorder()
	if !manager.Exchange(exchange, ticket, "llama") {
		t.Fatal("ticket exchange failed")
	}
	if manager.Exchange(httptest.NewRecorder(), ticket, "llama") {
		t.Fatal("ticket replay succeeded")
	}
	cookies := exchange.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("unexpected backend session cookie %#v", cookies)
	}
	request := httptest.NewRequest("GET", "https://backend/router/webuis/llama/", nil)
	request.AddCookie(cookies[0])
	if !manager.Authorized(request, "llama") || manager.Authorized(request, "sdcpp") {
		t.Fatal("backend session kind boundary failed")
	}
}

func TestProxyAccessTicketExpires(t *testing.T) {
	manager := newProxyAccessManager()
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	ticket := manager.Issue("llama")
	now = now.Add(time.Minute)
	if manager.Exchange(httptest.NewRecorder(), ticket, "llama") {
		t.Fatal("expired ticket succeeded")
	}
}
