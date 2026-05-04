package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestContentDefaultStatus(t *testing.T) {
	if got := contentDefaultStatus("auto_approve"); got != "approved" {
		t.Fatalf("expected approved got %s", got)
	}
	if got := contentDefaultStatus("manual_review"); got != "pending_review" {
		t.Fatalf("expected pending_review got %s", got)
	}
}

func TestAnonymousLabel(t *testing.T) {
	ts := time.Date(2026, 5, 4, 10, 30, 0, 0, time.UTC)
	if !strings.HasPrefix(anonymousLabel("tr", ts), "Anonim Kullanıcı") {
		t.Fatalf("unexpected tr label")
	}
	if !strings.HasPrefix(anonymousLabel("en", ts), "Anonymous User") {
		t.Fatalf("unexpected en label")
	}
}

func TestTermsRequiredValidation(t *testing.T) {
	s := &Server{}
	h := s.handleCreateEntry()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"type":"problem","category":"road","title":"A","description":"B","language":"tr","termsAccepted":false}`))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", res.Code)
	}
}
