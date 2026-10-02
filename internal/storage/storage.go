package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

type UploadResult struct {
	Bucket     string
	ObjectName string
	MimeType   string
	ByteSize   int64
}

type Store interface {
	UploadImage(ctx context.Context, objectName, mimeType string, data []byte) (UploadResult, error)
	SignedGetURL(ctx context.Context, bucket, objectName string, ttl time.Duration) (string, error)
}

// ErrGenerationMismatch means the object changed after it was read, so the
// write that wanted to replace that version was refused.
var ErrGenerationMismatch = errors.New("storage: object changed since it was read")

// ObjectStore keeps small private files next to the uploads. Only the GCS
// store has one; callers find it with a type assertion and do without it
// otherwise.
type ObjectStore interface {
	// ReadObject returns the object and its generation, or generation 0 and no
	// error when there is no such object.
	ReadObject(ctx context.Context, name string) ([]byte, int64, error)
	// WriteObject replaces the object only if it is still at generation, and
	// with generation 0 only if there is none yet. Otherwise it returns
	// ErrGenerationMismatch and leaves the object alone.
	WriteObject(ctx context.Context, name, contentType string, data []byte, generation int64) error
	DeleteObject(ctx context.Context, name string) error
}

type GCSStore struct {
	client *storage.Client
	bucket string
}

var _ ObjectStore = (*GCSStore)(nil)

func NewGCSStore(ctx context.Context, bucket string) (*GCSStore, error) {
	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GCSStore{client: c, bucket: bucket}, nil
}

func (s *GCSStore) UploadImage(ctx context.Context, objectName, mimeType string, data []byte) (UploadResult, error) {
	w := s.client.Bucket(s.bucket).Object(objectName).NewWriter(ctx)
	w.ContentType = mimeType
	if _, err := bytes.NewReader(data).WriteTo(w); err != nil {
		_ = w.Close()
		return UploadResult{}, err
	}
	if err := w.Close(); err != nil {
		return UploadResult{}, err
	}
	return UploadResult{Bucket: s.bucket, ObjectName: objectName, MimeType: mimeType, ByteSize: int64(len(data))}, nil
}

func (s *GCSStore) SignedGetURL(_ context.Context, bucket, objectName string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	u, err := s.client.Bucket(bucket).SignedURL(objectName, &storage.SignedURLOptions{
		Method:  http.MethodGet,
		Expires: time.Now().Add(ttl),
		Scheme:  storage.SigningSchemeV4,
	})
	if err != nil {
		return "", err
	}
	return u, nil
}

func (s *GCSStore) ReadObject(ctx context.Context, name string) ([]byte, int64, error) {
	r, err := s.client.Bucket(s.bucket).Object(name).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, err
	}
	return data, r.Attrs.Generation, nil
}

func (s *GCSStore) WriteObject(ctx context.Context, name, contentType string, data []byte, generation int64) error {
	cond := storage.Conditions{DoesNotExist: true}
	if generation != 0 {
		cond = storage.Conditions{GenerationMatch: generation}
	}
	w := s.client.Bucket(s.bucket).Object(name).If(cond).NewWriter(ctx)
	w.ContentType = contentType
	// One request and no buffer: the default keeps 16 MiB per write for a
	// retry, and callers already handle a write that fails.
	w.ChunkSize = 0
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	err := w.Close()
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusPreconditionFailed {
		return ErrGenerationMismatch
	}
	return err
}

func (s *GCSStore) DeleteObject(ctx context.Context, name string) error {
	// Deleting twice does no harm, so a transient error is retried until ctx ends.
	err := s.client.Bucket(s.bucket).Object(name).Retryer(storage.WithPolicy(storage.RetryAlways)).Delete(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	return err
}

func (s *GCSStore) Close() error { return s.client.Close() }

type LocalStore struct {
	baseDir, publicBaseURL string
}

func NewLocalStore(baseDir, publicBaseURL string) *LocalStore {
	return &LocalStore{baseDir: baseDir, publicBaseURL: strings.TrimRight(publicBaseURL, "/")}
}

func (l *LocalStore) UploadImage(_ context.Context, objectName, mimeType string, data []byte) (UploadResult, error) {
	path := filepath.Join(l.baseDir, objectName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return UploadResult{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return UploadResult{}, err
	}
	return UploadResult{Bucket: "local", ObjectName: objectName, MimeType: mimeType, ByteSize: int64(len(data))}, nil
}

func (l *LocalStore) SignedGetURL(_ context.Context, _ string, objectName string, _ time.Duration) (string, error) {
	return fmt.Sprintf("%s/%s", l.publicBaseURL, strings.TrimLeft(objectName, "/")), nil
}

func RandomObjectName(prefix, ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	safePrefix := strings.Trim(strings.TrimSpace(prefix), "/")
	if safePrefix == "" {
		safePrefix = "yalinli"
	}
	return fmt.Sprintf("%s/%s%s", safePrefix, hex.EncodeToString(b), ext), nil
}

func EncodeStorageRef(bucket, objectName string) string {
	return fmt.Sprintf("gcs://%s/%s", bucket, objectName)
}

func ParseStorageRef(v string) (bucket, objectName string, ok bool) {
	if !strings.HasPrefix(v, "gcs://") {
		return "", "", false
	}
	rest := strings.TrimPrefix(v, "gcs://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
