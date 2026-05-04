package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBFFSecret(t *testing.T) {
	h := BFFSecret("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-BFF-SECRET", "secret")
	res2 := httptest.NewRecorder()
	h.ServeHTTP(res2, req2)
	if res2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res2.Code)
	}
}

func TestLanguageResolver(t *testing.T) {
	h := LanguageResolver("tr")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(LangFrom(r.Context())))
	}))
	req := httptest.NewRequest(http.MethodGet, "/?lang=en", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if got := res.Body.String(); got != "en" {
		t.Fatalf("expected en got %s", got)
	}
}
