package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yalinli-api/internal/auth"
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

func TestRequireAuthRejectsRefreshToken(t *testing.T) {
	tokens := auth.NewTokenManager("secret", time.Minute, time.Hour)
	refresh, _, err := tokens.Mint("user-1", "refresh")
	if err != nil {
		t.Fatal(err)
	}
	h := RequireAuth(tokens)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+refresh)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
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

func TestCORSDoesNotShortCircuitV1Options(t *testing.T) {
	called := false
	h := CORS([]string{"https://yalinli.org"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusForbidden)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
	req.Header.Set("Origin", "https://yalinli.org")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if !called {
		t.Fatal("expected /v1 OPTIONS to continue to route middleware")
	}
}
