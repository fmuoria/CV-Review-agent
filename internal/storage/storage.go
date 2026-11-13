// Package storage keeps uploaded CVs and cover letters.
//
// Two backends:
//   - Local: a folder (on Render, a persistent disk mounted at e.g. /var/data).
//   - GCS:   a private Google Cloud Storage bucket, using the same service
//     account as Vertex AI. Better for backups and if you later run more than
//     one server instance.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/googleauth"
)

// ErrNotFound means the object does not exist.
var ErrNotFound = errors.New("file not found")

// Store saves and loads files by key (e.g. "applications/42/cv-1.pdf").
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// Local stores files under a directory.
type Local struct{ Dir string }

func (l Local) path(key string) (string, error) {
	clean := filepath.Clean("/" + key) // strip any ../
	if clean == "/" {
		return "", errors.New("empty storage key")
	}
	return filepath.Join(l.Dir, clean), nil
}

// Put implements Store.
func (l Local) Put(_ context.Context, key string, data []byte, _ string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Get implements Store.
func (l Local) Get(_ context.Context, key string) ([]byte, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

// Delete implements Store.
func (l Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// GCS stores files in a Cloud Storage bucket via the JSON API.
type GCS struct {
	Bucket string
	Tokens googleauth.TokenSource
	HTTP   *http.Client
}

func (g GCS) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (g GCS) do(ctx context.Context, method, u string, body []byte, contentType string) ([]byte, int, error) {
	tok, err := g.Tokens.Token(ctx)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 100<<20))
	return b, resp.StatusCode, err
}

// Put implements Store.
func (g GCS) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	u := fmt.Sprintf("https://storage.googleapis.com/upload/storage/v1/b/%s/o?uploadType=media&name=%s",
		url.PathEscape(g.Bucket), url.QueryEscape(key))
	b, status, err := g.do(ctx, http.MethodPost, u, data, contentType)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("GCS upload failed (%d): %s", status, strings.TrimSpace(string(b)))
	}
	return nil
}

// Get implements Store.
func (g GCS) Get(ctx context.Context, key string) ([]byte, error) {
	u := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s/o/%s?alt=media",
		url.PathEscape(g.Bucket), url.PathEscape(key))
	b, status, err := g.do(ctx, http.MethodGet, u, nil, "")
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		return b, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("GCS download failed (%d)", status)
}

// Delete implements Store.
func (g GCS) Delete(ctx context.Context, key string) error {
	u := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s/o/%s",
		url.PathEscape(g.Bucket), url.PathEscape(key))
	_, status, err := g.do(ctx, http.MethodDelete, u, nil, "")
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("GCS delete failed (%d)", status)
	}
	return nil
}
