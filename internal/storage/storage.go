package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
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

type GCSStore struct {
	client *storage.Client
	bucket string
}

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

func RandomObjectName(prefix, ext string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	safePrefix := strings.Trim(strings.TrimSpace(prefix), "/")
	if safePrefix == "" {
		safePrefix = "yalinli"
	}
	return fmt.Sprintf("%s/%s%s", safePrefix, hex.EncodeToString(b), ext)
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
