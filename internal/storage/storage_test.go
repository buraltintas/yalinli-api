package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeGCS answers the few JSON/XML API calls the object methods make, with
// generations and ifGenerationMatch the way GCS applies them.
type fakeGCS struct {
	mu   sync.Mutex
	data []byte
	gen  int64
}

func (f *fakeGCS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	const name = "_cache/feed-v1.json"
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/bucket/"+name || r.URL.Query().Get("alt") == "media"):
		if f.gen == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(f.data)))
		w.Header().Set("X-Goog-Generation", strconv.FormatInt(f.gen, 10))
		w.Header().Set("X-Goog-Metageneration", "1")
		_, _ = w.Write(f.data)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/upload/storage/v1/b/bucket/o"):
		if want := r.URL.Query().Get("ifGenerationMatch"); want != strconv.FormatInt(f.gen, 10) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = io.WriteString(w, `{"error":{"code":412,"message":"At least one of the pre-conditions you specified did not hold.","errors":[{"reason":"conditionNotMet"}]}}`)
			return
		}
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		if _, err := mr.NextPart(); err != nil { // object metadata
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		part, err := mr.NextPart()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.data, _ = io.ReadAll(part)
		f.gen++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"bucket":"bucket","name":%q,"generation":"%d","metageneration":"1","size":"%d"}`, name, f.gen, len(f.data))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/storage/v1/b/bucket/o/"):
		if f.gen == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":404,"message":"No such object"}}`)
			return
		}
		f.data, f.gen = nil, 0
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.String(), http.StatusNotImplemented)
	}
}

func TestGCSObjectGenerations(t *testing.T) {
	srv := httptest.NewServer(&fakeGCS{})
	defer srv.Close()
	t.Setenv("STORAGE_EMULATOR_HOST", srv.URL)
	ctx := context.Background()
	st, err := NewGCSStore(ctx, "bucket")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const name = "_cache/feed-v1.json"

	if data, gen, err := st.ReadObject(ctx, name); err != nil || gen != 0 || data != nil {
		t.Fatalf("missing object: %q %d %v", data, gen, err)
	}
	if err := st.WriteObject(ctx, name, "application/json", []byte(`{"a":1}`), 0); err != nil {
		t.Fatalf("first write: %v", err)
	}
	data, gen, err := st.ReadObject(ctx, name)
	if err != nil || gen != 1 || string(data) != `{"a":1}` {
		t.Fatalf("read back: %q %d %v", data, gen, err)
	}
	if err := st.WriteObject(ctx, name, "application/json", []byte(`{"a":2}`), 0); !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("write expecting no object: %v", err)
	}
	if err := st.WriteObject(ctx, name, "application/json", []byte(`{"a":2}`), 7); !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("write with a stale generation: %v", err)
	}
	if err := st.WriteObject(ctx, name, "application/json", []byte(`{"a":3}`), gen); err != nil {
		t.Fatalf("write with the current generation: %v", err)
	}
	if data, gen, _ := st.ReadObject(ctx, name); gen != 2 || string(data) != `{"a":3}` {
		t.Fatalf("after replace: %q %d", data, gen)
	}
	if err := st.DeleteObject(ctx, name); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := st.DeleteObject(ctx, name); err != nil {
		t.Fatalf("delete of a missing object: %v", err)
	}
}
