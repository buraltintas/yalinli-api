package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"

	"yalinli-api/internal/auth"
	"yalinli-api/internal/config"
	"yalinli-api/internal/email"
	"yalinli-api/internal/http/middleware"
	"yalinli-api/internal/i18n"
	"yalinli-api/internal/response"
	"yalinli-api/internal/storage"
	"yalinli-api/internal/validation"
)

type Server struct {
	cfg    config.Config
	logger *slog.Logger
	db     *pgxpool.Pool
	tokens *auth.TokenManager
	emails *email.Service
	store  storage.Store
}

func NewRouter(cfg config.Config, logger *slog.Logger, db *pgxpool.Pool) http.Handler {
	s := &Server{cfg: cfg, logger: logger, db: db, tokens: auth.NewTokenManager(cfg.JWTSecret, config.AccessTTL(cfg), config.RefreshTTL(cfg)), emails: email.NewService(logger, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.EmailFrom, cfg.EmailFromName), store: storage.NewLocalStore(cfg.StorageLocalDir, cfg.MediaPublicBaseURL)}
	return NewRouterWithStore(cfg, logger, db, s.store)
}

func NewRouterWithStore(cfg config.Config, logger *slog.Logger, db *pgxpool.Pool, st storage.Store) http.Handler {
	s := &Server{cfg: cfg, logger: logger, db: db, tokens: auth.NewTokenManager(cfg.JWTSecret, config.AccessTTL(cfg), config.RefreshTTL(cfg)), emails: email.NewService(logger, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.EmailFrom, cfg.EmailFromName), store: st}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.LanguageResolver(cfg.DefaultLanguage), middleware.Logging(logger), middleware.Recover(logger), middleware.CORS(cfg.CORSAllowedOrigins), middleware.MaxBodyBytes(cfg.MaxBodyBytes))
	if cfg.StorageBackend == "local" {
		r.Get("/public/media/{name}", s.handlePublicMedia())
	}
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(middleware.BFFSecret(cfg.BFFSecret))
		v1.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})
		v1.Post("/auth/signup/request-code", s.rateByIP(rate.Every(time.Minute/time.Duration(max(cfg.RateLimitOTPPerMin, 1))), cfg.RateLimitOTPPerMin, s.handleSignupRequestCode()))
		v1.Post("/auth/login/request-code", s.rateByIP(rate.Every(time.Minute/time.Duration(max(cfg.RateLimitOTPPerMin, 1))), cfg.RateLimitOTPPerMin, s.handleLoginRequestCode()))
		v1.Post("/auth/verify-code", s.handleVerifyCode())
		v1.Post("/auth/refresh", s.handleRefresh())
		v1.Post("/auth/logout", s.handleLogout())
		v1.Post("/unsubscribe", s.handleUnsubscribe())

		v1.Get("/feed", s.handleFeed())
		v1.Get("/entries/{id}", s.handleGetEntry())
		v1.Post("/entries", s.rateByIP(rate.Every(10*time.Minute/time.Duration(max(cfg.RateLimitEntryPer10Min, 1))), cfg.RateLimitEntryPer10Min, s.handleCreateEntry()))
		v1.Get("/entries/{id}/comments", s.handleGetComments())
		v1.Post("/entries/{id}/comments", s.rateByIP(rate.Every(10*time.Minute/time.Duration(max(cfg.RateLimitCommentPer10Min, 1))), cfg.RateLimitCommentPer10Min, s.handleCreateComment()))
		v1.Post("/media/upload", s.rateByIP(rate.Every(time.Hour/time.Duration(max(cfg.RateLimitMediaPerHour, 1))), cfg.RateLimitMediaPerHour, s.handleMediaUpload()))

		v1.Get("/petitions", s.handlePetitions())
		v1.Get("/petitions/{id}", s.handlePetition())
		v1.Post("/reports", s.rateByIP(rate.Every(10*time.Minute/time.Duration(max(cfg.RateLimitReportPer10Min, 1))), cfg.RateLimitReportPer10Min, s.handleReport()))

		v1.Group(func(a chi.Router) {
			a.Use(middleware.RequireAuth(s.tokens))
			a.Get("/me", s.handleMe())
			a.Patch("/me", s.handlePatchMe())
			a.Get("/me/notification-preferences", s.handleGetNotificationPreferences())
			a.Patch("/me/notification-preferences", s.handlePatchNotificationPreferences())
			a.Post("/petitions", s.handleCreatePetition())
			a.Post("/petitions/{id}/sign", s.handleSignPetition())
			a.Get("/petitions/{id}/export-data", s.rateByIP(rate.Every(time.Minute/2), 2, s.handlePetitionExportData()))
			a.Delete("/petitions/{id}/sign", s.handleUnsignPetition())
		})
	})
	return r
}

func (s *Server) rateByIP(rps rate.Limit, burst int, next http.HandlerFunc) http.HandlerFunc {
	return middleware.RateLimit(rps, max(burst, 1))(next).ServeHTTP
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func hash(sv string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sv)))
	return hex.EncodeToString(sum[:])
}
func contentDefaultStatus(mode string) string {
	if mode == "manual_review" {
		return "pending_review"
	}
	return "approved"
}
func lang(r *http.Request) string { return middleware.LangFrom(r.Context()) }

func (s *Server) handleSignupRequestCode() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email string `json:"email"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		em := validation.NormalizeEmail(in.Email)
		if err := validation.ValidateEmail(em); err != nil {
			badInput(w, r, "invalid_input")
			return
		}
		var exists bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, em).Scan(&exists)
		if exists {
			response.Error(w, http.StatusConflict, "user_already_exists", i18n.Message(lang(r), "user_already_exists"))
			return
		}
		if _, err := s.db.Exec(r.Context(), `INSERT INTO users(email,preferred_language) VALUES ($1,'tr') ON CONFLICT DO NOTHING`, em); err != nil {
			internalErr(w, r)
			return
		}
		s.issueCode(r.Context(), em, "signup", lang(r))
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) handleLoginRequestCode() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email string `json:"email"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		em := validation.NormalizeEmail(in.Email)
		if err := validation.ValidateEmail(em); err != nil {
			badInput(w, r, "invalid_input")
			return
		}
		var exists bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, em).Scan(&exists)
		if !exists {
			response.Error(w, http.StatusNotFound, "user_not_found", i18n.Message(lang(r), "user_not_found"))
			return
		}
		s.issueCode(r.Context(), em, "login", lang(r))
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) issueCode(ctx context.Context, emailAddr, purpose, lang string) {
	code, _ := auth.GenerateNumericCode()
	hashed, _ := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	expires := time.Now().UTC().Add(time.Duration(s.cfg.OTPCodeTTLMinutes) * time.Minute)
	_, _ = s.db.Exec(ctx, `INSERT INTO auth_codes(email,purpose,code_hash,expires_at,max_attempts) VALUES($1,$2,$3,$4,$5)`, emailAddr, purpose, string(hashed), expires, s.cfg.OTPMaxAttempts)
	s.emails.SendAsync(ctx, emailAddr, email.LoginCodeSubject(lang), email.LoginCodeBody(lang, code, s.cfg.OTPCodeTTLMinutes))
}

func (s *Server) handleVerifyCode() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email   string `json:"email"`
			Code    string `json:"code"`
			Purpose string `json:"purpose"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		em := validation.NormalizeEmail(in.Email)
		row := s.db.QueryRow(r.Context(), `SELECT id,code_hash,expires_at,attempt_count,max_attempts,consumed_at FROM auth_codes WHERE email=$1 AND purpose=$2 ORDER BY created_at DESC LIMIT 1`, em, strings.TrimSpace(in.Purpose))
		var id uuid.UUID
		var codeHash string
		var expires time.Time
		var attempts, maxAttempts int
		var consumed *time.Time
		if err := row.Scan(&id, &codeHash, &expires, &attempts, &maxAttempts, &consumed); err != nil {
			response.Error(w, http.StatusUnauthorized, "unauthorized", i18n.Message(lang(r), "unauthorized"))
			return
		}
		if consumed != nil || time.Now().UTC().After(expires) || attempts >= maxAttempts {
			response.Error(w, http.StatusUnauthorized, "unauthorized", i18n.Message(lang(r), "unauthorized"))
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(strings.TrimSpace(in.Code))); err != nil {
			_, _ = s.db.Exec(r.Context(), `UPDATE auth_codes SET attempt_count=attempt_count+1 WHERE id=$1`, id)
			response.Error(w, http.StatusUnauthorized, "unauthorized", i18n.Message(lang(r), "unauthorized"))
			return
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE auth_codes SET consumed_at=now() WHERE id=$1`, id)
		var userID string
		if err := s.db.QueryRow(r.Context(), `SELECT id FROM users WHERE email=$1`, em).Scan(&userID); err != nil {
			internalErr(w, r)
			return
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE users SET email_verified_at=now(),updated_at=now() WHERE id=$1`, userID)
		access, _, _ := s.tokens.Mint(userID, "access")
		refresh, refreshExp, _ := s.tokens.Mint(userID, "refresh")
		refreshHash := hash(refresh)
		_, _ = s.db.Exec(r.Context(), `INSERT INTO refresh_tokens(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, userID, refreshHash, refreshExp)
		response.JSON(w, http.StatusOK, map[string]any{"accessToken": access, "refreshToken": refresh})
	}
}

func (s *Server) handleRefresh() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			RefreshToken string `json:"refreshToken"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		claims, err := s.tokens.Parse(strings.TrimSpace(in.RefreshToken))
		if err != nil || claims.TokenType != "refresh" {
			response.Error(w, http.StatusUnauthorized, "unauthorized", i18n.Message(lang(r), "unauthorized"))
			return
		}
		tHash := hash(in.RefreshToken)
		var exists bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE user_id=$1 AND token_hash=$2 AND revoked_at IS NULL AND expires_at>now())`, claims.UserID, tHash).Scan(&exists)
		if !exists {
			response.Error(w, http.StatusUnauthorized, "unauthorized", i18n.Message(lang(r), "unauthorized"))
			return
		}
		access, _, _ := s.tokens.Mint(claims.UserID, "access")
		response.JSON(w, http.StatusOK, map[string]any{"accessToken": access})
	}
}

func (s *Server) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			RefreshToken string `json:"refreshToken"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE refresh_tokens SET revoked_at=now() WHERE token_hash=$1`, hash(in.RefreshToken))
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) handleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFrom(r.Context())
		row := s.db.QueryRow(r.Context(), `SELECT id,email,display_name,preferred_language,email_notifications_enabled,notify_new_entries,notify_resolved_problems,notify_new_petitions,notify_new_events,notify_announcements FROM users WHERE id=$1`, uid)
		var out struct {
			ID, Email                                                                                          string
			DisplayName                                                                                        *string
			PreferredLanguage                                                                                  string
			EmailNotificationsEnabled                                                                          bool
			NotifyNewEntries, NotifyResolvedProblems, NotifyNewPetitions, NotifyNewEvents, NotifyAnnouncements bool
		}
		if err := row.Scan(&out.ID, &out.Email, &out.DisplayName, &out.PreferredLanguage, &out.EmailNotificationsEnabled, &out.NotifyNewEntries, &out.NotifyResolvedProblems, &out.NotifyNewPetitions, &out.NotifyNewEvents, &out.NotifyAnnouncements); err != nil {
			internalErr(w, r)
			return
		}
		response.JSON(w, http.StatusOK, out)
	}
}
func (s *Server) handlePatchMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFrom(r.Context())
		var in struct {
			DisplayName               *string `json:"displayName"`
			PreferredLanguage         *string `json:"preferredLanguage"`
			EmailNotificationsEnabled *bool   `json:"emailNotificationsEnabled"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if in.PreferredLanguage != nil && !validation.ValidLang(*in.PreferredLanguage) {
			badInput(w, r, "invalid_input")
			return
		}
		_, err := s.db.Exec(r.Context(), `UPDATE users SET display_name=COALESCE($2,display_name), preferred_language=COALESCE($3,preferred_language), email_notifications_enabled=COALESCE($4,email_notifications_enabled), updated_at=now() WHERE id=$1`, uid, in.DisplayName, in.PreferredLanguage, in.EmailNotificationsEnabled)
		if err != nil {
			internalErr(w, r)
			return
		}
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
func (s *Server) handleGetNotificationPreferences() http.HandlerFunc { return s.handleMe() }
func (s *Server) handlePatchNotificationPreferences() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFrom(r.Context())
		var in struct{ NotifyNewEntries, NotifyResolvedProblems, NotifyNewPetitions, NotifyNewEvents, NotifyAnnouncements *bool }
		if !decodeJSON(w, r, &in) {
			return
		}
		_, err := s.db.Exec(r.Context(), `UPDATE users SET notify_new_entries=COALESCE($2,notify_new_entries),notify_resolved_problems=COALESCE($3,notify_resolved_problems),notify_new_petitions=COALESCE($4,notify_new_petitions),notify_new_events=COALESCE($5,notify_new_events),notify_announcements=COALESCE($6,notify_announcements),updated_at=now() WHERE id=$1`, uid, in.NotifyNewEntries, in.NotifyResolvedProblems, in.NotifyNewPetitions, in.NotifyNewEvents, in.NotifyAnnouncements)
		if err != nil {
			internalErr(w, r)
			return
		}
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
func (s *Server) handleUnsubscribe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email string `json:"email"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE users SET email_notifications_enabled=false,updated_at=now() WHERE email=$1`, validation.NormalizeEmail(in.Email))
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) handleCreateEntry() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Type, Category, Title, Description, Language string
			DisplayName                                  *string
			IsAnonymous                                  bool
			ContactEmail                                 *string
			LocationText                                 *string
			EventDate                                    *time.Time
			MediaIDs                                     []string
			TermsAccepted                                bool
			PrivacyAccepted                              bool
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if !in.TermsAccepted {
			response.Error(w, http.StatusBadRequest, "terms_not_accepted", i18n.Message(lang(r), "terms_not_accepted"))
			return
		}
		if strings.TrimSpace(in.Title) == "" || len([]rune(in.Title)) > 160 || strings.TrimSpace(in.Description) == "" || len([]rune(in.Description)) > 4000 || !validation.ValidLang(in.Language) {
			badInput(w, r, "invalid_input")
			return
		}
		if len(in.MediaIDs) > s.cfg.MediaMaxImagesPerEntry {
			badInput(w, r, "invalid_input")
			return
		}
		for _, mid := range in.MediaIDs {
			if _, err := uuid.Parse(strings.TrimSpace(mid)); err != nil {
				badInput(w, r, "invalid_input")
				return
			}
		}
		uid, ok := middleware.UserIDFrom(r.Context())
		var userID *string
		if ok {
			userID = &uid
		}
		status := contentDefaultStatus(s.cfg.ContentReviewMode)
		tx, err := s.db.Begin(r.Context())
		if err != nil {
			internalErr(w, r)
			return
		}
		defer tx.Rollback(r.Context())
		var id string
		err = tx.QueryRow(r.Context(), `INSERT INTO entries(type,category,title,description,language,status,user_id,display_name,is_anonymous,anonymous_display_date,contact_email,location_text,event_date,terms_accepted_at,terms_version,privacy_accepted_at,privacy_version,ip_hash,user_agent_hash,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),$10,$11,$12,now(),$13,CASE WHEN $14 THEN now() ELSE NULL END,CASE WHEN $14 THEN $15 ELSE NULL END,$16,$17,now()) RETURNING id`, in.Type, in.Category, in.Title, in.Description, in.Language, status, userID, in.DisplayName, in.IsAnonymous, in.ContactEmail, in.LocationText, in.EventDate, s.cfg.CurrentTermsVersion, in.PrivacyAccepted, s.cfg.CurrentPrivacyVersion, hash(r.RemoteAddr), hash(r.UserAgent())).Scan(&id)
		if err != nil {
			internalErr(w, r)
			return
		}
		if len(in.MediaIDs) > 0 {
			tag, err := tx.Exec(r.Context(), `UPDATE media SET entry_id=$2 WHERE id = ANY($1::uuid[]) AND entry_id IS NULL AND status='approved'`, in.MediaIDs, id)
			if err != nil {
				internalErr(w, r)
				return
			}
			if int(tag.RowsAffected()) != len(in.MediaIDs) {
				badInput(w, r, "invalid_input")
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			internalErr(w, r)
			return
		}
		response.JSON(w, http.StatusCreated, map[string]any{"id": id, "status": status})
	}
}

func (s *Server) handleFeed() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := `SELECT e.id,e.type,e.category,e.title,left(e.description,280),e.language,e.status,e.display_name,e.is_anonymous,e.created_at,(SELECT COUNT(*) FROM comments c WHERE c.entry_id=e.id AND c.status IN ('approved','reported')),COALESCE((SELECT array_agg(m.url ORDER BY m.created_at ASC) FROM media m WHERE m.entry_id=e.id AND m.status='approved'),ARRAY[]::text[]) FROM entries e WHERE e.status IN ('approved','reported')`
		args := []any{}
		if t := strings.TrimSpace(r.URL.Query().Get("type")); t != "" {
			args = append(args, t)
			q += fmt.Sprintf(" AND e.type=$%d", len(args))
		}
		if c := strings.TrimSpace(r.URL.Query().Get("category")); c != "" {
			args = append(args, c)
			q += fmt.Sprintf(" AND e.category=$%d", len(args))
		}
		if l := strings.TrimSpace(r.URL.Query().Get("lang")); l != "" {
			args = append(args, l)
			q += fmt.Sprintf(" AND e.language=$%d", len(args))
		}
		q += " ORDER BY e.created_at DESC LIMIT 50"
		rows, err := s.db.Query(r.Context(), q, args...)
		if err != nil {
			internalErr(w, r)
			return
		}
		defer rows.Close()
		type item struct {
			ID, Type, Category, Title, DescriptionPreview, Language, Status, AuthorDisplay string
			CreatedAt                                                                      time.Time
			CommentCount                                                                   int
			ImageURLs                                                                      []string `json:"imageUrls"`
		}
		out := []item{}
		for rows.Next() {
			var it item
			var name *string
			var isAnon bool
			var images []string
			if err := rows.Scan(&it.ID, &it.Type, &it.Category, &it.Title, &it.DescriptionPreview, &it.Language, &it.Status, &name, &isAnon, &it.CreatedAt, &it.CommentCount, &images); err != nil {
				internalErr(w, r)
				return
			}
			it.ImageURLs = s.resolveImageRefs(r.Context(), images)
			if isAnon {
				it.AuthorDisplay = anonymousLabel(lang(r), it.CreatedAt)
			} else if name != nil {
				it.AuthorDisplay = strings.TrimSpace(*name)
			}
			if it.AuthorDisplay == "" {
				it.AuthorDisplay = "Yalinli User"
			}
			out = append(out, it)
		}
		response.JSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

func anonymousLabel(l string, ts time.Time) string {
	if l == "en" {
		return "Anonymous User " + ts.Local().Format("02.01.2006 15:04")
	}
	return "Anonim Kullanıcı " + ts.Local().Format("02.01.2006 15:04")
}

func (s *Server) handleGetEntry() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		row := s.db.QueryRow(r.Context(), `SELECT id,type,category,title,description,language,status,display_name,is_anonymous,created_at FROM entries WHERE id=$1`, id)
		var outID, outType, outCategory, outTitle, outDescription, outLanguage, outStatus string
		var name *string
		var isAnon bool
		var created time.Time
		if err := row.Scan(&outID, &outType, &outCategory, &outTitle, &outDescription, &outLanguage, &outStatus, &name, &isAnon, &created); err != nil {
			response.Error(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		out := map[string]any{
			"id":          outID,
			"type":        outType,
			"category":    outCategory,
			"title":       outTitle,
			"description": outDescription,
			"language":    outLanguage,
			"status":      outStatus,
		}
		if isAnon {
			out["authorDisplay"] = anonymousLabel(lang(r), created)
		} else if name != nil {
			out["authorDisplay"] = *name
		}
		rows, err := s.db.Query(r.Context(), `SELECT url FROM media WHERE entry_id=$1 AND status='approved' ORDER BY created_at ASC LIMIT $2`, id, s.cfg.MediaMaxImagesPerEntry)
		if err == nil {
			defer rows.Close()
			refs := []string{}
			for rows.Next() {
				var ref string
				_ = rows.Scan(&ref)
				refs = append(refs, ref)
			}
			out["imageUrls"] = s.resolveImageRefs(r.Context(), refs)
		}
		response.JSON(w, http.StatusOK, out)
	}
}
func (s *Server) handleGetComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		rows, err := s.db.Query(r.Context(), `SELECT id,body,language,display_name,is_anonymous,created_at,status FROM comments WHERE entry_id=$1 AND status IN ('approved','reported') ORDER BY created_at ASC`, id)
		if err != nil {
			internalErr(w, r)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var cid, body, langv, status string
			var name *string
			var isAnon bool
			var created time.Time
			_ = rows.Scan(&cid, &body, &langv, &name, &isAnon, &created, &status)
			author := ""
			if isAnon {
				author = anonymousLabel(lang(r), created)
			} else if name != nil {
				author = *name
			}
			out = append(out, map[string]any{"id": cid, "body": body, "language": langv, "authorDisplay": author, "status": status, "createdAt": created})
		}
		response.JSON(w, http.StatusOK, map[string]any{"items": out})
	}
}
func (s *Server) handleCreateComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eid := chi.URLParam(r, "id")
		var in struct {
			Body, Language string
			DisplayName    *string
			IsAnonymous    bool
			TermsAccepted  bool
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if !in.TermsAccepted {
			response.Error(w, http.StatusBadRequest, "terms_not_accepted", i18n.Message(lang(r), "terms_not_accepted"))
			return
		}
		if strings.TrimSpace(in.Body) == "" || len([]rune(in.Body)) > 2000 || !validation.ValidLang(in.Language) {
			badInput(w, r, "invalid_input")
			return
		}
		uid, ok := middleware.UserIDFrom(r.Context())
		var userID *string
		if ok {
			userID = &uid
		}
		status := contentDefaultStatus(s.cfg.ContentReviewMode)
		var cid string
		err := s.db.QueryRow(r.Context(), `INSERT INTO comments(entry_id,user_id,body,language,display_name,is_anonymous,anonymous_display_date,status,terms_accepted_at,terms_version,ip_hash,user_agent_hash,published_at) VALUES($1,$2,$3,$4,$5,$6,now(),$7,now(),$8,$9,$10,now()) RETURNING id`, eid, userID, in.Body, in.Language, in.DisplayName, in.IsAnonymous, status, s.cfg.CurrentTermsVersion, hash(r.RemoteAddr), hash(r.UserAgent())).Scan(&cid)
		if err != nil {
			internalErr(w, r)
			return
		}
		response.JSON(w, http.StatusCreated, map[string]any{"id": cid, "status": status})
	}
}

func (s *Server) handleMediaUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(s.cfg.MaxMultipartBytes); err != nil {
			badInput(w, r, "invalid_input")
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			badInput(w, r, "invalid_input")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, int64(s.cfg.MediaMaxFileSizeMB)*1024*1024+1))
		if err != nil || len(data) > s.cfg.MediaMaxFileSizeMB*1024*1024 {
			badInput(w, r, "invalid_input")
			return
		}
		mimeType, err := validation.ValidateImageContent(data)
		if err != nil {
			badInput(w, r, "invalid_input")
			return
		}
		ext := validation.ExtensionFromMIME(mimeType)
		objectName := storage.RandomObjectName(s.cfg.GCSUploadPrefix, ext)
		up, err := s.store.UploadImage(r.Context(), objectName, mimeType, data)
		if err != nil {
			internalErr(w, r)
			return
		}
		ref := storage.EncodeStorageRef(up.Bucket, up.ObjectName)
		url, err := s.resolveSingleImageURL(r.Context(), ref)
		if err != nil {
			internalErr(w, r)
			return
		}
		var mediaID string
		if err := s.db.QueryRow(r.Context(), `INSERT INTO media(entry_id,status,mime_type,url) VALUES(NULL,'approved',$1,$2) RETURNING id`, mimeType, ref).Scan(&mediaID); err != nil {
			internalErr(w, r)
			return
		}
		response.JSON(w, http.StatusCreated, map[string]any{"id": mediaID, "url": url})
	}
}

func (s *Server) handlePublicMedia() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}

func (s *Server) resolveImageRefs(ctx context.Context, refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		u, err := s.resolveSingleImageURL(ctx, ref)
		if err == nil && u != "" {
			out = append(out, u)
		}
	}
	return out
}

func (s *Server) resolveSingleImageURL(ctx context.Context, ref string) (string, error) {
	bucket, objectName, ok := storage.ParseStorageRef(ref)
	if !ok {
		return ref, nil
	}
	return s.store.SignedGetURL(ctx, bucket, objectName, s.cfg.GCSSignedURLTTL)
}

func (s *Server) handleCreatePetition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFrom(r.Context())
		var in struct {
			Category, Title, Description, Language string
			TargetAuthority                        *string
			RequestedAction                        *string
			LocationText                           *string
			DeadlineAt                             *time.Time
			TermsAccepted                          bool
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if !in.TermsAccepted {
			response.Error(w, http.StatusBadRequest, "terms_not_accepted", i18n.Message(lang(r), "terms_not_accepted"))
			return
		}
		status := contentDefaultStatus(s.cfg.ContentReviewMode)
		tx, err := s.db.Begin(r.Context())
		if err != nil {
			internalErr(w, r)
			return
		}
		defer tx.Rollback(r.Context())
		var entryID string
		err = tx.QueryRow(r.Context(), `INSERT INTO entries(type,category,title,description,language,status,user_id,is_anonymous,anonymous_display_date,terms_accepted_at,terms_version,ip_hash,user_agent_hash,published_at) VALUES('petition',$1,$2,$3,$4,$5,$6,false,now(),now(),$7,$8,$9,now()) RETURNING id`, in.Category, in.Title, in.Description, in.Language, status, uid, s.cfg.CurrentTermsVersion, hash(r.RemoteAddr), hash(r.UserAgent())).Scan(&entryID)
		if err != nil {
			internalErr(w, r)
			return
		}
		var pid string
		err = tx.QueryRow(r.Context(), `INSERT INTO petitions(entry_id,status,created_by,target_authority,requested_action,location_text,deadline_at,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()) RETURNING id`, entryID, status, uid, in.TargetAuthority, in.RequestedAction, in.LocationText, in.DeadlineAt).Scan(&pid)
		if err != nil {
			internalErr(w, r)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			internalErr(w, r)
			return
		}
		response.JSON(w, http.StatusCreated, map[string]any{"id": pid, "status": status})
	}
}
func (s *Server) handlePetitions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.db.Query(r.Context(), `SELECT p.id,e.title,e.description,p.signature_count,p.status,e.created_at FROM petitions p JOIN entries e ON e.id=p.entry_id WHERE p.status IN ('approved','closed') ORDER BY e.created_at DESC LIMIT 50`)
		if err != nil {
			internalErr(w, r)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, title, desc, status string
			var count int
			var created time.Time
			_ = rows.Scan(&id, &title, &desc, &count, &status, &created)
			out = append(out, map[string]any{"id": id, "title": title, "description": desc, "signatureCount": count, "status": status, "createdAt": created})
		}
		response.JSON(w, http.StatusOK, map[string]any{"items": out})
	}
}
func (s *Server) handlePetition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		row := s.db.QueryRow(r.Context(), `SELECT p.id,e.title,e.description,p.signature_count,p.status,e.created_at FROM petitions p JOIN entries e ON e.id=p.entry_id WHERE p.id=$1 AND p.status NOT IN ('hidden','deleted')`, id)
		var pid, title, desc, status string
		var count int
		var created time.Time
		if err := row.Scan(&pid, &title, &desc, &count, &status, &created); err != nil {
			response.Error(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		masked := []string{}
		signRows, err := s.db.Query(r.Context(), `SELECT COALESCE(masked_signer_name,'') FROM petition_signatures WHERE petition_id=$1 ORDER BY signed_at DESC LIMIT 20`, pid)
		if err == nil {
			defer signRows.Close()
			for signRows.Next() {
				var m string
				_ = signRows.Scan(&m)
				if strings.TrimSpace(m) != "" {
					masked = append(masked, m)
				}
			}
		}
		var viewerSigned bool
		if uid, ok := middleware.UserIDFrom(r.Context()); ok {
			_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM petition_signatures WHERE petition_id=$1 AND user_id=$2)`, pid, uid).Scan(&viewerSigned)
		}
		response.JSON(w, http.StatusOK, map[string]any{"id": pid, "title": title, "description": desc, "signatureCount": count, "status": status, "createdAt": created, "maskedSigners": masked, "viewerSigned": viewerSigned})
	}
}
func (s *Server) handleSignPetition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFrom(r.Context())
		pid := chi.URLParam(r, "id")
		var in struct {
			Visibility    string
			TermsAccepted bool
			FullName      string
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if !in.TermsAccepted {
			response.Error(w, http.StatusBadRequest, "terms_not_accepted", i18n.Message(lang(r), "terms_not_accepted"))
			return
		}
		if in.Visibility != "public_name" && in.Visibility != "anonymous" {
			badInput(w, r, "invalid_input")
			return
		}
		var pStatus string
		if err := s.db.QueryRow(r.Context(), `SELECT status FROM petitions WHERE id=$1`, pid).Scan(&pStatus); err != nil {
			response.Error(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if pStatus != "approved" {
			response.Error(w, http.StatusBadRequest, "invalid_input", "petition not signable")
			return
		}
		tx, err := s.db.Begin(r.Context())
		if err != nil {
			internalErr(w, r)
			return
		}
		defer tx.Rollback(r.Context())
		var email string
		var profileName *string
		if err := tx.QueryRow(r.Context(), `SELECT email, full_name FROM users WHERE id=$1`, uid).Scan(&email, &profileName); err != nil {
			internalErr(w, r)
			return
		}
		fullName := strings.TrimSpace(in.FullName)
		if fullName == "" && profileName != nil {
			fullName = strings.TrimSpace(*profileName)
		}
		if fullName == "" {
			badInput(w, r, "invalid_input")
			return
		}
		signedAt := time.Now().UTC()
		masked := maskFullName(fullName, lang(r))
		sigHash := makeSignatureHash(s.cfg.SignatureHashSecret, pid, uid, signedAt, email, s.cfg.CurrentTermsVersion)
		_, err = tx.Exec(r.Context(), `INSERT INTO petition_signatures(petition_id,user_id,visibility,terms_accepted_at,terms_version,signer_full_name_snapshot,signer_email_snapshot,masked_signer_name,signed_at,signature_hash,updated_at) VALUES($1,$2,$3,now(),$4,$5,$6,$7,$8,$9,now())`, pid, uid, in.Visibility, s.cfg.CurrentTermsVersion, fullName, email, masked, signedAt, sigHash)
		if err != nil {
			response.Error(w, http.StatusConflict, "PETITION_ALREADY_SIGNED", "already signed")
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE petitions SET signature_count=signature_count+1,updated_at=now() WHERE id=$1`, pid)
		if err != nil {
			internalErr(w, r)
			return
		}
		_ = tx.Commit(r.Context())
		response.JSON(w, http.StatusCreated, map[string]any{"ok": true})
	}
}
func (s *Server) handleUnsignPetition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFrom(r.Context())
		pid := chi.URLParam(r, "id")
		tx, _ := s.db.Begin(r.Context())
		defer tx.Rollback(r.Context())
		tag, _ := tx.Exec(r.Context(), `DELETE FROM petition_signatures WHERE petition_id=$1 AND user_id=$2`, pid, uid)
		if tag.RowsAffected() > 0 {
			_, _ = tx.Exec(r.Context(), `UPDATE petitions SET signature_count=GREATEST(signature_count-1,0),updated_at=now() WHERE id=$1`, pid)
		}
		_ = tx.Commit(r.Context())
		response.JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) handlePetitionExportData() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFrom(r.Context())
		pid := chi.URLParam(r, "id")

		var out struct {
			PetitionID      string
			Title           string
			Description     string
			TargetAuthority *string
			RequestedAction *string
			CreatedAt       time.Time
			CreatedByID     string
			CreatedByEmail  string
			CreatedByName   *string
			SignatureCount  int
			Status          string
		}
		err := s.db.QueryRow(r.Context(), `SELECT p.id,e.title,e.description,p.target_authority,p.requested_action,p.created_at,p.created_by,u.email,u.full_name,p.signature_count,p.status FROM petitions p JOIN entries e ON e.id=p.entry_id JOIN users u ON u.id=p.created_by WHERE p.id=$1`, pid).
			Scan(&out.PetitionID, &out.Title, &out.Description, &out.TargetAuthority, &out.RequestedAction, &out.CreatedAt, &out.CreatedByID, &out.CreatedByEmail, &out.CreatedByName, &out.SignatureCount, &out.Status)
		if err != nil {
			response.Error(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if out.Status == "hidden" || out.Status == "deleted" {
			response.Error(w, http.StatusForbidden, "forbidden", i18n.Message(lang(r), "forbidden"))
			return
		}
		if out.CreatedByID != uid {
			response.Error(w, http.StatusForbidden, "forbidden", i18n.Message(lang(r), "forbidden"))
			return
		}

		rows, err := s.db.Query(r.Context(), `SELECT signer_full_name_snapshot, signer_email_snapshot, signed_at, signature_hash FROM petition_signatures WHERE petition_id=$1 ORDER BY signed_at ASC`, pid)
		if err != nil {
			internalErr(w, r)
			return
		}
		defer rows.Close()
		type sigItem struct {
			RowNumber               int       `json:"rowNumber"`
			SignerFullName          string    `json:"signerFullName"`
			SignerEmail             string    `json:"signerEmail"`
			SignedAt                time.Time `json:"signedAt"`
			ElectronicApprovalText  string    `json:"electronicApprovalText"`
			WetSignatureLabel       string    `json:"wetSignatureLabel"`
			WetSignaturePlaceholder string    `json:"wetSignaturePlaceholder"`
			SignatureHash           string    `json:"signatureHash,omitempty"`
		}
		sigs := []sigItem{}
		i := 1
		for rows.Next() {
			var fullName, emailSnapshot, sigHash string
			var signedAt time.Time
			_ = rows.Scan(&fullName, &emailSnapshot, &signedAt, &sigHash)
			sigs = append(sigs, sigItem{
				RowNumber:               i,
				SignerFullName:          fullName,
				SignerEmail:             emailSnapshot,
				SignedAt:                signedAt,
				ElectronicApprovalText:  electronicApprovalText(lang(r)),
				WetSignatureLabel:       wetSignatureLabel(lang(r)),
				WetSignaturePlaceholder: "",
				SignatureHash:           sigHash,
			})
			i++
		}
		_, _ = s.db.Exec(r.Context(), `INSERT INTO petition_export_logs(petition_id,exported_by_user_id,signature_count_at_export,exported_at,ip_hash,user_agent_hash) VALUES($1,$2,$3,now(),$4,$5)`, pid, uid, len(sigs), hash(r.RemoteAddr), hash(r.UserAgent()))

		response.JSON(w, http.StatusOK, map[string]any{
			"petitionId":      out.PetitionID,
			"title":           out.Title,
			"description":     out.Description,
			"targetAuthority": out.TargetAuthority,
			"requestedAction": out.RequestedAction,
			"createdBy": map[string]any{
				"id":       out.CreatedByID,
				"email":    out.CreatedByEmail,
				"fullName": out.CreatedByName,
			},
			"createdAt":      out.CreatedAt,
			"exportedAt":     time.Now().UTC(),
			"signatureCount": len(sigs),
			"signatures":     sigs,
			"legalNote":      legalNote(lang(r)),
		})
	}
}

func (s *Server) handleReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			TargetType, TargetID, Reason string
			Description, ReporterEmail   *string
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if in.TargetType == "" || in.TargetID == "" || in.Reason == "" {
			badInput(w, r, "invalid_input")
			return
		}
		tid, err := uuid.Parse(in.TargetID)
		if err != nil {
			badInput(w, r, "invalid_input")
			return
		}
		_, err = s.db.Exec(r.Context(), `INSERT INTO reports(target_type,target_id,reason,description,reporter_email,ip_hash,user_agent_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`, in.TargetType, tid, in.Reason, in.Description, in.ReporterEmail, hash(r.RemoteAddr), hash(r.UserAgent()))
		if err != nil {
			internalErr(w, r)
			return
		}
		if in.TargetType == "entry" {
			_, _ = s.db.Exec(r.Context(), `UPDATE entries SET status='reported',updated_at=now() WHERE id=$1`, tid)
		}
		if in.TargetType == "comment" {
			_, _ = s.db.Exec(r.Context(), `UPDATE comments SET status='reported',updated_at=now() WHERE id=$1`, tid)
		}
		if s.cfg.AdminContactEmail != "" {
			s.emails.SendAsync(r.Context(), s.cfg.AdminContactEmail, "New report", "A content report was submitted.")
		}
		response.JSON(w, http.StatusCreated, map[string]any{"ok": true})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_input", "invalid input")
		return false
	}
	return true
}
func badInput(w http.ResponseWriter, r *http.Request, code string) {
	response.Error(w, http.StatusBadRequest, code, i18n.Message(lang(r), code))
}
func internalErr(w http.ResponseWriter, r *http.Request) {
	response.Error(w, http.StatusInternalServerError, "internal_error", i18n.Message(lang(r), "internal_error"))
}

func maskFullName(fullName, language string) string {
	parts := strings.Fields(strings.TrimSpace(fullName))
	if len(parts) < 2 {
		if language == "en" {
			return "Masked User"
		}
		return "Maskeli Kullanıcı"
	}
	mask := func(s string) string {
		r := []rune(s)
		if len(r) == 0 {
			return ""
		}
		if len(r) == 1 {
			return string(r)
		}
		return string(r[0]) + strings.Repeat("*", len(r)-1)
	}
	return mask(parts[0]) + " " + mask(parts[1])
}

func electronicApprovalText(language string) string {
	if language == "en" {
		return "Electronically approved through yalinli.org."
	}
	return "yalinli.org üzerinden elektronik olarak onaylanmıştır."
}

func wetSignatureLabel(language string) string {
	if language == "en" {
		return "Wet Signature"
	}
	return "Islak İmza"
}

func legalNote(language string) string {
	if language == "en" {
		return "This record does not replace a qualified electronic signature or wet signature; it indicates electronic approval given through yalinli.org."
	}
	return "Bu kayıt, nitelikli elektronik imza veya ıslak imza yerine geçmez; kampanyaya yalinli.org üzerinden verilen elektronik onayı gösterir."
}

func makeSignatureHash(secret, petitionID, userID string, signedAt time.Time, signerEmail, termsVersion string) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte(strings.Join([]string{
		petitionID,
		userID,
		signedAt.UTC().Format(time.RFC3339Nano),
		strings.ToLower(strings.TrimSpace(signerEmail)),
		termsVersion,
	}, "|")))
	return hex.EncodeToString(m.Sum(nil))
}
