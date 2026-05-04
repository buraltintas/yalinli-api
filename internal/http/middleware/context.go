package middleware

import "context"

type ctxKey string

const (
	requestIDKey ctxKey = "request_id"
	userIDKey    ctxKey = "user_id"
	langKey      ctxKey = "lang"
)

func WithRequestID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, requestIDKey, v)
}
func RequestIDFrom(ctx context.Context) string { s, _ := ctx.Value(requestIDKey).(string); return s }
func WithUserID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, userIDKey, v)
}
func UserIDFrom(ctx context.Context) (string, bool) {
	s, ok := ctx.Value(userIDKey).(string)
	return s, ok
}
func WithLang(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, langKey, v)
}
func LangFrom(ctx context.Context) string {
	s, _ := ctx.Value(langKey).(string)
	if s == "" {
		return "tr"
	}
	return s
}
