package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"yalinli-api/internal/config"
	"yalinli-api/internal/storage"
)

// memObjects is an in-memory ObjectStore with the same generation rules as
// the GCS one.
type memObjects struct {
	mu      sync.Mutex
	data    map[string][]byte
	gen     map[string]int64
	next    int64
	reads   int
	writes  int
	readErr error
	// stallReads and stallWrites make those calls wait until their context
	// ends, like a store that has stopped answering.
	stallReads, stallWrites bool
	// beforeWrite runs before a write is checked, so a test can change the
	// object underneath it.
	beforeWrite func(m *memObjects)
}

func newMemObjects() *memObjects {
	return &memObjects{data: map[string][]byte{}, gen: map[string]int64{}}
}

func (m *memObjects) ReadObject(ctx context.Context, name string) ([]byte, int64, error) {
	if m.stallReads {
		<-ctx.Done()
		return nil, 0, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	if m.readErr != nil {
		return nil, 0, m.readErr
	}
	return m.data[name], m.gen[name], nil
}

func (m *memObjects) WriteObject(ctx context.Context, name, _ string, data []byte, generation int64) error {
	if m.stallWrites {
		<-ctx.Done()
		return ctx.Err()
	}
	if hook := m.beforeWrite; hook != nil {
		m.beforeWrite = nil
		hook(m)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gen[name] != generation {
		return storage.ErrGenerationMismatch
	}
	m.writes++
	m.next++
	m.data[name] = append([]byte(nil), data...)
	m.gen[name] = m.next
	return nil
}

func (m *memObjects) DeleteObject(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, name)
	delete(m.gen, name)
	return nil
}

func (m *memObjects) put(t *testing.T, snap feedSnapshot) {
	t.Helper()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	m.data[feedSnapshotObject] = data
	m.gen[feedSnapshotObject] = m.next
}

func (m *memObjects) snapshot(t *testing.T) (feedSnapshot, bool) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.data[feedSnapshotObject]
	if !ok {
		return feedSnapshot{}, false
	}
	var snap feedSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	return snap, true
}

// memStore signs URLs like the local store and keeps objects in memory.
type memStore struct {
	*storage.LocalStore
	*memObjects
}

func newMemStore() memStore {
	return memStore{LocalStore: storage.NewLocalStore("", "http://media.test"), memObjects: newMemObjects()}
}

func testConfig() config.Config {
	return config.Config{
		BFFSecret:                "bff",
		JWTSecret:                "jwt",
		SignatureHashSecret:      "sig",
		JWTAccessTokenTTLMinutes: 60,
		JWTRefreshTokenTTLDays:   1,
		DefaultLanguage:          "tr",
		ContentReviewMode:        "auto_approve",
		CurrentTermsVersion:      "v1",
		CurrentPrivacyVersion:    "v1",
		MediaMaxImagesPerEntry:   3,
		MaxBodyBytes:             1 << 20,
		StorageBackend:           "gcs",
		GCSSignedURLTTL:          time.Hour,
		FeedSnapshotMaxAge:       6 * time.Hour,
		RateLimitEntryPer10Min:   100,
		RateLimitCommentPer10Min: 100,
		RateLimitReportPer10Min:  100,
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// unreachablePool fails every query, like a database that cannot be reached.
func unreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func getFeed(t *testing.T, h http.Handler, query, acceptLanguage string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/feed"+query, nil)
	req.Header.Set("X-BFF-SECRET", "bff")
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res.Code, res.Body.String()
}

func sampleSnapshot(builtAt time.Time) feedSnapshot {
	name := "Ayşe"
	created := time.Date(2026, 8, 17, 9, 30, 0, 0, time.UTC)
	return feedSnapshot{BuiltAt: builtAt, Items: []feedRow{
		{ID: "e1", Type: "problem", Category: "infrastructure", Title: "Yol", DescriptionPreview: "Çukur", Language: "tr", Status: "approved", DisplayName: &name, CreatedAt: created, CommentCount: 2, ImageRefs: []string{"gcs://bucket/yalinli/a.jpg"}},
		{ID: "e2", Type: "beauty", Category: "environment", Title: "Çiçek", DescriptionPreview: "Bahar", Language: "tr", Status: "reported", IsAnonymous: true, CreatedAt: created, ImageRefs: []string{}},
	}}
}

func TestFeedFromFreshSnapshotDoesNotTouchDatabase(t *testing.T) {
	st := newMemStore()
	st.put(t, sampleSnapshot(time.Now().UTC().Add(-time.Hour)))
	// No pool at all: any query would panic and fail the test.
	h := NewRouterWithStore(testConfig(), discardLogger(), nil, st)

	code, body := getFeed(t, h, "", "en")
	if code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", code, body)
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("expected 2 items got %d", len(out.Items))
	}
	first, second := out.Items[0], out.Items[1]
	if first["ID"] != "e1" || first["AuthorDisplay"] != "Ayşe" || first["CommentCount"] != float64(2) || first["Status"] != "approved" {
		t.Fatalf("unexpected first item: %v", first)
	}
	if urls, _ := first["imageUrls"].([]any); len(urls) != 1 || urls[0] != "http://media.test/yalinli/a.jpg" {
		t.Fatalf("image ref was not signed per request: %v", first["imageUrls"])
	}
	if label, _ := second["AuthorDisplay"].(string); !strings.HasPrefix(label, "Anonymous User") {
		t.Fatalf("anonymous label not in the request language: %q", label)
	}
	if st.writes != 0 {
		t.Fatalf("a fresh snapshot was rewritten")
	}
}

func TestFilteredFeedSkipsSnapshot(t *testing.T) {
	st := newMemStore()
	st.put(t, sampleSnapshot(time.Now().UTC()))
	h := NewRouterWithStore(testConfig(), discardLogger(), unreachablePool(t), st)

	for _, q := range []string{"?type=problem", "?category=infrastructure", "?lang=tr"} {
		if code, _ := getFeed(t, h, q, ""); code != http.StatusInternalServerError {
			t.Fatalf("%s: expected the database path (500 with no database) got %d", q, code)
		}
	}
	if st.reads != 0 {
		t.Fatalf("filtered feed read the snapshot %d times", st.reads)
	}
}

func TestExpiredSnapshotServedWhenDatabaseUnreachable(t *testing.T) {
	st := newMemStore()
	st.put(t, sampleSnapshot(time.Now().UTC().Add(-7*time.Hour)))
	h := NewRouterWithStore(testConfig(), discardLogger(), unreachablePool(t), st)

	code, body := getFeed(t, h, "", "")
	if code != http.StatusOK || !strings.Contains(body, `"ID":"e1"`) {
		t.Fatalf("expected the expired snapshot got %d: %s", code, body)
	}
	if st.writes != 0 {
		t.Fatalf("snapshot was overwritten without fresh rows")
	}
}

func TestMissingSnapshotAndUnreachableDatabaseIsAnError(t *testing.T) {
	st := newMemStore()
	h := NewRouterWithStore(testConfig(), discardLogger(), unreachablePool(t), st)

	if code, _ := getFeed(t, h, "", ""); code != http.StatusInternalServerError {
		t.Fatalf("expected 500 got %d", code)
	}
	if _, ok := st.snapshot(t); ok {
		t.Fatalf("a snapshot was written without rows")
	}
}

func TestRefreshDeletesSnapshotWhenDatabaseUnreachable(t *testing.T) {
	st := newMemStore()
	st.put(t, sampleSnapshot(time.Now().UTC()))
	s := &Server{cfg: testConfig(), logger: discardLogger(), db: unreachablePool(t), store: st, snapshots: st}

	s.refreshFeedSnapshot(context.Background())
	if _, ok := st.snapshot(t); ok {
		t.Fatalf("snapshot kept although it could not be rebuilt after a write")
	}
}

func TestRefreshDeletesSnapshotWhenStoreStalls(t *testing.T) {
	st := newMemStore()
	st.put(t, sampleSnapshot(time.Now().UTC()))
	st.stallReads = true
	s := &Server{cfg: testConfig(), logger: discardLogger(), db: unreachablePool(t), store: st, snapshots: st}

	start := time.Now()
	s.refreshFeedSnapshot(context.Background())
	if _, ok := st.snapshot(t); ok {
		t.Fatalf("snapshot kept although the store stalled during the refresh")
	}
	if took := time.Since(start); took > 2*feedSnapshotTimeout {
		t.Fatalf("refresh held the write for %s", took)
	}
}

func TestSnapshotOffWithoutObjectStoreOrMaxAge(t *testing.T) {
	cfg := testConfig()
	cfg.FeedSnapshotMaxAge = 0
	st := newMemStore()
	st.put(t, sampleSnapshot(time.Now().UTC()))
	h := NewRouterWithStore(cfg, discardLogger(), unreachablePool(t), st)
	if code, _ := getFeed(t, h, "", ""); code != http.StatusInternalServerError || st.reads != 0 {
		t.Fatalf("FEED_SNAPSHOT_MAX_AGE=0 should keep the feed on the database, got %d with %d reads", code, st.reads)
	}

	h = NewRouterWithStore(testConfig(), discardLogger(), unreachablePool(t), storage.NewLocalStore("", "http://media.test"))
	if code, _ := getFeed(t, h, "", ""); code != http.StatusInternalServerError {
		t.Fatalf("local store should keep the feed on the database, got %d", code)
	}
}
