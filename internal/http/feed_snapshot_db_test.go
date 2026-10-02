package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"yalinli-api/internal/auth"
	"yalinli-api/internal/storage"
)

// These tests need a throwaway Postgres on this machine, never the one in
// .env: they migrate it and empty its tables. For example
//
//	initdb -D $TMPDIR/pg && pg_ctl -D $TMPDIR/pg -o "-p 55432" start
//	createdb -p 55432 yalinli_test
//	YALINLI_TEST_DATABASE_URL=postgres://localhost:55432/yalinli_test?sslmode=disable go test ./internal/http/
func testDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("YALINLI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("YALINLI_TEST_DATABASE_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if h := cfg.ConnConfig.Host; h != "localhost" && h != "127.0.0.1" && h != "::1" && !strings.HasPrefix(h, "/") {
		t.Fatalf("refusing to empty a database on %s; point YALINLI_TEST_DATABASE_URL at a local throwaway one", h)
	}
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), `TRUNCATE users, auth_codes, refresh_tokens, entries, media, comments, petitions, petition_signatures, petition_export_logs, reports, email_logs CASCADE`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func seedEntry(t *testing.T, pool *pgxpool.Pool, title string, displayName *string, anonymous bool, created time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `INSERT INTO entries(type,category,title,description,language,status,display_name,is_anonymous,terms_accepted_at,terms_version,created_at) VALUES('problem','infrastructure',$1,'Açıklama','tr','approved',$2,$3,now(),'v1',$4) RETURNING id`, title, displayName, anonymous, created).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func send(t *testing.T, h http.Handler, method, path, body, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-BFF-SECRET", "bff")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	out := map[string]any{}
	_ = json.Unmarshal(res.Body.Bytes(), &out)
	return res.Code, out
}

func feedItemByID(t *testing.T, body, id string) map[string]any {
	t.Helper()
	var out struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	for _, it := range out.Items {
		if it["ID"] == id {
			return it
		}
	}
	t.Fatalf("entry %s is not in the feed: %s", id, body)
	return nil
}

func TestFeedSnapshotMatchesDatabaseAndFollowsWrites(t *testing.T) {
	pool := testDatabase(t)
	ctx := context.Background()
	name, secret := "Ayşe", "Gizli İsim"
	now := time.Now().UTC()
	named := seedEntry(t, pool, "Yol çukuru", &name, false, now.Add(-2*time.Hour))
	seedEntry(t, pool, "Park", &secret, true, now.Add(-time.Hour))
	if _, err := pool.Exec(ctx, `INSERT INTO media(entry_id,status,mime_type,url) VALUES($1,'approved','image/jpeg','gcs://bucket/yalinli/a.jpg')`, named); err != nil {
		t.Fatal(err)
	}

	st := newMemStore()
	h := NewRouterWithStore(testConfig(), discardLogger(), pool, st)
	plain := NewRouterWithStore(testConfig(), discardLogger(), pool, storage.NewLocalStore("", "http://media.test"))

	// The first read builds the snapshot; later reads must answer exactly what
	// the database path answers, in both languages.
	if code, _ := getFeed(t, h, "", ""); code != http.StatusOK || st.writes != 1 {
		t.Fatalf("first read: code %d, writes %d", code, st.writes)
	}
	for _, accept := range []string{"", "en"} {
		_, fromDB := getFeed(t, plain, "", accept)
		_, fromSnapshot := getFeed(t, h, "", accept)
		if fromDB != fromSnapshot {
			t.Fatalf("snapshot and database differ (%q):\n%s\n%s", accept, fromSnapshot, fromDB)
		}
	}
	if st.writes != 1 {
		t.Fatalf("fresh snapshot rewritten on read: %d writes", st.writes)
	}
	raw := string(st.data[feedSnapshotObject])
	if !strings.Contains(raw, "gcs://bucket/yalinli/a.jpg") || strings.Contains(raw, "http://media.test") {
		t.Fatalf("snapshot must keep storage refs, not signed URLs: %s", raw)
	}
	if strings.Contains(raw, secret) {
		t.Fatalf("snapshot keeps an anonymous entry's name: %s", raw)
	}

	// A row written straight into the database (as hand moderation in SQL
	// would) does not show while the snapshot is fresh: reads do not query.
	seedEntry(t, pool, "Elle eklenen", nil, false, now.Add(-30*time.Minute))
	if _, body := getFeed(t, h, "", ""); strings.Contains(body, "Elle eklenen") {
		t.Fatalf("fresh snapshot read went to the database")
	}

	// Writes through the API rebuild it before they respond.
	code, created := send(t, h, http.MethodPost, "/v1/entries", `{"type":"problem","category":"infrastructure","title":"Yeni kayıt","description":"Açıklama","language":"tr","termsAccepted":true}`, "")
	if code != http.StatusCreated {
		t.Fatalf("create entry: %d %v", code, created)
	}
	entryID, _ := created["id"].(string)
	_, body := getFeed(t, h, "", "")
	feedItemByID(t, body, entryID)
	if !strings.Contains(body, "Elle eklenen") {
		t.Fatalf("rebuild after a write did not read the database")
	}

	if code, out := send(t, h, http.MethodPost, "/v1/entries/"+entryID+"/comments", `{"body":"Yorum","language":"tr","termsAccepted":true}`, ""); code != http.StatusCreated {
		t.Fatalf("create comment: %d %v", code, out)
	}
	_, body = getFeed(t, h, "", "")
	if it := feedItemByID(t, body, entryID); it["CommentCount"] != float64(1) {
		t.Fatalf("comment count not refreshed: %v", it)
	}

	if code, out := send(t, h, http.MethodPost, "/v1/reports", `{"targetType":"entry","targetId":"`+entryID+`","reason":"spam"}`, ""); code != http.StatusCreated {
		t.Fatalf("report: %d %v", code, out)
	}
	_, body = getFeed(t, h, "", "")
	if it := feedItemByID(t, body, entryID); it["Status"] != "reported" {
		t.Fatalf("reported status not refreshed: %v", it)
	}

	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,full_name) VALUES('dilekce@example.test','Test Kullanıcı') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	token, _, err := auth.NewTokenManager("jwt", time.Hour, time.Hour).Mint(userID, "access")
	if err != nil {
		t.Fatal(err)
	}
	if code, out := send(t, h, http.MethodPost, "/v1/petitions", `{"category":"infrastructure","title":"Dilekçe başlığı","description":"Açıklama","language":"tr","termsAccepted":true}`, token); code != http.StatusCreated {
		t.Fatalf("create petition: %d %v", code, out)
	}
	if _, body := getFeed(t, h, "", ""); !strings.Contains(body, "Dilekçe başlığı") {
		t.Fatalf("petition entry not on the feed after it was created")
	}

	// Past the max age the next read rebuilds from the database.
	snap, _ := st.snapshot(t)
	snap.BuiltAt = now.Add(-7 * time.Hour)
	st.put(t, snap)
	seedEntry(t, pool, "Süresi dolan", nil, false, now.Add(-10*time.Minute))
	writes := st.writes
	if _, body := getFeed(t, h, "", ""); !strings.Contains(body, "Süresi dolan") || st.writes != writes+1 {
		t.Fatalf("expired snapshot was not rebuilt: writes %d -> %d", writes, st.writes)
	}
}

func TestSnapshotReadErrorFallsBackToDatabase(t *testing.T) {
	pool := testDatabase(t)
	seedEntry(t, pool, "Yol çukuru", nil, false, time.Now().UTC())
	st := newMemStore()
	st.readErr = errors.New("storage unavailable")
	h := NewRouterWithStore(testConfig(), discardLogger(), pool, st)

	code, body := getFeed(t, h, "", "")
	if code != http.StatusOK || !strings.Contains(body, "Yol çukuru") {
		t.Fatalf("expected the database feed got %d: %s", code, body)
	}
	if st.writes != 0 {
		t.Fatalf("wrote a snapshot without knowing its generation")
	}
}

func TestOlderRebuildCannotReplaceNewerSnapshot(t *testing.T) {
	pool := testDatabase(t)
	seedEntry(t, pool, "Yol çukuru", nil, false, time.Now().UTC())
	st := newMemStore()
	h := NewRouterWithStore(testConfig(), discardLogger(), pool, st)

	// A reader found no snapshot and queried; a write lands its own rebuild
	// before the reader stores what it read.
	newer := sampleSnapshot(time.Now().UTC())
	st.beforeWrite = func(m *memObjects) { m.put(t, newer) }
	if code, body := getFeed(t, h, "", ""); code != http.StatusOK || !strings.Contains(body, "Yol çukuru") {
		t.Fatalf("reader should still answer from the database, got %d: %s", code, body)
	}
	if snap, _ := st.snapshot(t); len(snap.Items) != len(newer.Items) || snap.Items[0].ID != newer.Items[0].ID {
		t.Fatalf("an older read replaced the newer snapshot: %+v", snap.Items)
	}

	// The same race on the write side is retried, so the write is not lost.
	s := &Server{cfg: testConfig(), logger: discardLogger(), db: pool, store: st, snapshots: st}
	st.beforeWrite = func(m *memObjects) { m.put(t, newer) }
	s.refreshFeedSnapshot(context.Background())
	if snap, ok := st.snapshot(t); !ok || len(snap.Items) != 1 || snap.Items[0].Title != "Yol çukuru" {
		t.Fatalf("refresh after a write did not land: %+v", snap.Items)
	}
}
