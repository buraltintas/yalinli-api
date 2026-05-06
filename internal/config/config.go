package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                   string
	Port                     string
	DatabaseURL              string
	MigrationsDatabaseURL    string
	BFFSecret                string
	JWTSecret                string
	SignatureHashSecret      string
	JWTAccessTokenTTLMinutes int
	JWTRefreshTokenTTLDays   int
	CORSAllowedOrigins       []string
	DefaultLanguage          string
	SupportedLanguages       map[string]bool
	ContentReviewMode        string
	CurrentTermsVersion      string
	CurrentPrivacyVersion    string
	PublicSiteURL            string
	PublicSiteName           string
	EmailFrom                string
	EmailFromName            string
	SMTPHost                 string
	SMTPPort                 int
	SMTPUsername             string
	SMTPPassword             string
	SMTPUseTLS               bool
	OTPCodeTTLMinutes        int
	OTPMaxAttempts           int
	MediaMaxImagesPerEntry   int
	MediaMaxFileSizeMB       int
	StorageBackend           string
	GCPProjectID             string
	GCSBucket                string
	GCSUploadPrefix          string
	GCSSignedURLTTL          time.Duration
	StorageLocalDir          string
	MediaPublicBaseURL       string
	AdminContactEmail        string
	MaxBodyBytes             int64
	MaxMultipartBytes        int64
	LogLevel                 slog.Level
	RateLimitEntryPer10Min   int
	RateLimitCommentPer10Min int
	RateLimitMediaPerHour    int
	RateLimitReportPer10Min  int
	RateLimitOTPPerMin       int
	RateLimitOTPPerDay       int
}

func Load() (Config, error) {
	if _, err := os.Stat(".env"); err == nil {
		_ = godotenv.Load(".env")
	}
	cfg := Config{
		AppEnv:                   getDefault("APP_ENV", "development"),
		Port:                     getDefault("PORT", "8080"),
		DatabaseURL:              strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MigrationsDatabaseURL:    getDefault("MIGRATIONS_DATABASE_URL", strings.TrimSpace(os.Getenv("DATABASE_URL"))),
		BFFSecret:                strings.TrimSpace(os.Getenv("BFF_SECRET")),
		JWTSecret:                strings.TrimSpace(os.Getenv("JWT_SECRET")),
		SignatureHashSecret:      strings.TrimSpace(os.Getenv("SIGNATURE_HASH_SECRET")),
		JWTAccessTokenTTLMinutes: parseIntDefault("JWT_ACCESS_TOKEN_TTL_MINUTES", 60),
		JWTRefreshTokenTTLDays:   parseIntDefault("JWT_REFRESH_TOKEN_TTL_DAYS", 30),
		CORSAllowedOrigins:       parseCSV(getDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		DefaultLanguage:          getDefault("DEFAULT_LANGUAGE", "tr"),
		SupportedLanguages:       mapFromCSV(getDefault("SUPPORTED_LANGUAGES", "tr,en")),
		ContentReviewMode:        getDefault("CONTENT_REVIEW_MODE", "auto_approve"),
		CurrentTermsVersion:      getDefault("CURRENT_TERMS_VERSION", "2026-05-04-v1"),
		CurrentPrivacyVersion:    getDefault("CURRENT_PRIVACY_VERSION", "2026-05-04-v1"),
		PublicSiteURL:            getDefault("PUBLIC_SITE_URL", "https://yalinli.org"),
		PublicSiteName:           getDefault("PUBLIC_SITE_NAME", "yalinli.org"),
		EmailFrom:                getDefault("EMAIL_FROM", "noreply@example.com"),
		EmailFromName:            getDefault("EMAIL_FROM_NAME", "Yalinli"),
		SMTPHost:                 strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:                 parseIntDefault("SMTP_PORT", 587),
		SMTPUsername:             strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:             strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		SMTPUseTLS:               parseBoolDefault("SMTP_USE_TLS", true),
		OTPCodeTTLMinutes:        parseIntDefault("OTP_CODE_TTL_MINUTES", 10),
		OTPMaxAttempts:           parseIntDefault("OTP_MAX_ATTEMPTS", 5),
		MediaMaxImagesPerEntry:   parseIntDefault("MEDIA_MAX_IMAGES_PER_ENTRY", 3),
		MediaMaxFileSizeMB:       parseIntDefault("MEDIA_MAX_FILE_SIZE_MB", 5),
		StorageBackend:           getDefault("STORAGE_BACKEND", "gcs"),
		GCPProjectID:             getDefault("GCP_PROJECT_ID", ""),
		GCSBucket:                getDefault("GCS_BUCKET", ""),
		GCSUploadPrefix:          getDefault("GCS_UPLOAD_PREFIX", "yalinli"),
		GCSSignedURLTTL:          parseDurationDefault("GCS_SIGNED_URL_TTL", 24*time.Hour),
		StorageLocalDir:          getDefault("STORAGE_LOCAL_DIR", "./var/storage"),
		MediaPublicBaseURL:       getDefault("MEDIA_PUBLIC_BASE_URL", "http://localhost:8080/public/media"),
		AdminContactEmail:        strings.TrimSpace(os.Getenv("ADMIN_CONTACT_EMAIL")),
		MaxBodyBytes:             parseInt64Default("MAX_BODY_BYTES", 1<<20),
		MaxMultipartBytes:        parseInt64Default("MAX_MULTIPART_BYTES", 20<<20),
		LogLevel:                 parseLogLevel(getDefault("LOG_LEVEL", "info")),
		RateLimitEntryPer10Min:   parseIntDefault("RATE_LIMIT_ENTRY_PER_10MIN", 3),
		RateLimitCommentPer10Min: parseIntDefault("RATE_LIMIT_COMMENT_PER_10MIN", 10),
		RateLimitMediaPerHour:    parseIntDefault("RATE_LIMIT_MEDIA_PER_HOUR", 10),
		RateLimitReportPer10Min:  parseIntDefault("RATE_LIMIT_REPORT_PER_10MIN", 5),
		RateLimitOTPPerMin:       parseIntDefault("RATE_LIMIT_OTP_PER_MIN", 1),
		RateLimitOTPPerDay:       parseIntDefault("RATE_LIMIT_OTP_PER_DAY", 10),
	}
	if err := requireEnv("DATABASE_URL", cfg.DatabaseURL); err != nil {
		return Config{}, err
	}
	if err := requireEnv("BFF_SECRET", cfg.BFFSecret); err != nil {
		return Config{}, err
	}
	if err := requireEnv("JWT_SECRET", cfg.JWTSecret); err != nil {
		return Config{}, err
	}
	if err := requireEnv("SIGNATURE_HASH_SECRET", cfg.SignatureHashSecret); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = "8080"
	}
	if cfg.StorageBackend == "gcs" {
		if cfg.GCSBucket == "" {
			return Config{}, fmt.Errorf("GCS_BUCKET is required when STORAGE_BACKEND=gcs")
		}
		if cfg.GCSUploadPrefix == "" {
			return Config{}, fmt.Errorf("GCS_UPLOAD_PREFIX is required when STORAGE_BACKEND=gcs")
		}
		if cfg.GCSSignedURLTTL <= 0 {
			return Config{}, fmt.Errorf("GCS_SIGNED_URL_TTL must be > 0")
		}
	} else if cfg.StorageBackend != "local" {
		return Config{}, fmt.Errorf("STORAGE_BACKEND must be one of: gcs, local")
	}
	return cfg, nil
}

func getDefault(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func requireEnv(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
func parseIntDefault(k string, d int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}
func parseInt64Default(k string, d int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return d
	}
	return n
}
func parseBoolDefault(k string, d bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return d
	}
	return b
}
func parseCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
func mapFromCSV(v string) map[string]bool {
	m := map[string]bool{}
	for _, p := range parseCSV(v) {
		m[p] = true
	}
	return m
}
func parseLogLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
func parseDurationDefault(k string, d time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	dd, err := time.ParseDuration(v)
	if err != nil {
		return d
	}
	return dd
}
func AccessTTL(c Config) time.Duration {
	return time.Duration(c.JWTAccessTokenTTLMinutes) * time.Minute
}
func RefreshTTL(c Config) time.Duration {
	return time.Duration(c.JWTRefreshTokenTTLDays) * 24 * time.Hour
}
