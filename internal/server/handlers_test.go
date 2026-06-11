package server

import (
	"context"
	"majmun/internal/config/proxy"
	"majmun/internal/ctxutil"
	"majmun/internal/hashid"
	"majmun/internal/httpclient"
	"majmun/internal/listing"
	"majmun/internal/shell"
	"majmun/internal/urlgen"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type stubProvider struct{}

func (stubProvider) ID() string                           { return hashid.New("stub") }
func (stubProvider) Name() string                         { return "stub" }
func (stubProvider) Type() string                         { return "channel" }
func (stubProvider) URLGenerator() *urlgen.Generator      { return nil }
func (stubProvider) ExpiredLinkStreamer() *shell.Streamer { return nil }
func (stubProvider) ProxyConfig() proxy.Proxy             { return proxy.Proxy{} }
func (stubProvider) HTTPClient() listing.HTTPClient       { return httpclient.NewDirectClient(nil) }

func TestServeLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logo.png")
	content := []byte("\x89PNG\r\n\x1a\nfake-image-bytes")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	s := &Server{}
	rec := httptest.NewRecorder()
	s.serveLocalFile(context.Background(), rec, path)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if got := rec.Body.Bytes(); string(got) != string(content) {
		t.Errorf("body = %q, want %q", got, content)
	}
}

func TestServeLocalFileMissing(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.serveLocalFile(context.Background(), rec, filepath.Join(t.TempDir(), "nope.png"))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServeLocalFileDirectory(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.serveLocalFile(context.Background(), rec, t.TempDir())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleFileProxyRemoteURL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("remote-logo-bytes"))
	}))
	defer upstream.Close()

	ctx := ctxutil.WithProvider(context.Background(), stubProvider{})
	data := &urlgen.Data{
		RequestType: urlgen.RequestTypeFile,
		File:        urlgen.FileData{URL: upstream.URL + "/logo.png"},
	}

	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleFileProxy(ctx, rec, data)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "remote-logo-bytes" {
		t.Errorf("body = %q, want remote-logo-bytes", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}

func TestHandleFileProxyUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	ctx := ctxutil.WithProvider(context.Background(), stubProvider{})
	data := &urlgen.Data{
		RequestType: urlgen.RequestTypeFile,
		File:        urlgen.FileData{URL: upstream.URL + "/logo.png"},
	}

	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleFileProxy(ctx, rec, data)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestIsHTTPURL(t *testing.T) {
	cases := map[string]bool{
		"http://example.com/l.png":  true,
		"https://example.com/l.png": true,
		"/media/logos/cnn.png":      false,
		"logo.png":                  false,
		"file:///media/l.png":       false,
	}
	for in, want := range cases {
		if got := isHTTPURL(in); got != want {
			t.Errorf("isHTTPURL(%q) = %v, want %v", in, got, want)
		}
	}
}
