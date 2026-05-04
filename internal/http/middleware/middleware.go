package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"yalinli-api/internal/auth"
	"yalinli-api/internal/i18n"
	"yalinli-api/internal/response"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), reqID)))
	})
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(sw, r)
			logger.Info("request", "request_id", RequestIDFrom(r.Context()), "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration_ms", time.Since(start).Milliseconds())
		})
	}
}

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", "request_id", RequestIDFrom(r.Context()))
					response.Error(w, http.StatusInternalServerError, "internal_error", i18n.Message(LangFrom(r.Context()), "internal_error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func BFFSecret(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimSpace(r.Header.Get("X-BFF-SECRET")) != secret {
				response.Error(w, http.StatusForbidden, "forbidden", i18n.Message(LangFrom(r.Context()), "forbidden"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAuth(tokens *auth.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := strings.TrimSpace(r.Header.Get("Authorization"))
			if !strings.HasPrefix(h, "Bearer ") {
				response.Error(w, http.StatusUnauthorized, "unauthorized", i18n.Message(LangFrom(r.Context()), "unauthorized"))
				return
			}
			claims, err := tokens.Parse(strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")))
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "unauthorized", i18n.Message(LangFrom(r.Context()), "unauthorized"))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), claims.UserID)))
		})
	}
}

func CORS(allowed []string) func(http.Handler) http.Handler {
	allowedSet := map[string]bool{}
	for _, o := range allowed {
		allowedSet[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowedSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-BFF-SECRET,X-Request-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func LanguageResolver(defaultLang string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lang := strings.TrimSpace(r.URL.Query().Get("lang"))
			if lang != "tr" && lang != "en" {
				accept := strings.ToLower(r.Header.Get("Accept-Language"))
				if strings.HasPrefix(accept, "en") {
					lang = "en"
				} else {
					lang = defaultLang
				}
			}
			next.ServeHTTP(w, r.WithContext(WithLang(r.Context(), lang)))
		})
	}
}

type ipLimiter struct {
	mu sync.Mutex
	m  map[string]*rate.Limiter
	r  rate.Limit
	b  int
}

func newIPLimiter(r rate.Limit, b int) *ipLimiter {
	return &ipLimiter{m: map[string]*rate.Limiter{}, r: r, b: b}
}
func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.m[ip]
	if !ok {
		lim = rate.NewLimiter(l.r, l.b)
		l.m[ip] = lim
	}
	return lim.Allow()
}

func RateLimit(rps rate.Limit, burst int) func(http.Handler) http.Handler {
	lim := newIPLimiter(rps, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !lim.allow(clientIP(r)) {
				response.Error(w, http.StatusTooManyRequests, "rate_limited", i18n.Message(LangFrom(r.Context()), "rate_limited"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
