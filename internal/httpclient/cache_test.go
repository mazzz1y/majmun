package httpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	t.Run("successfully creates store", func(t *testing.T) {
		tmpDir := t.TempDir()

		st, err := NewStore(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer st.Close()

		if st.dir != tmpDir {
			t.Errorf("expected dir %s, got %s", tmpDir, st.dir)
		}
		if st.cleanupTicker == nil {
			t.Error("expected cleanupTicker to be set")
		}
		if st.doneCh == nil {
			t.Error("expected doneCh to be set")
		}
	})

	t.Run("creates store directory", func(t *testing.T) {
		tmpDir := filepath.Join(t.TempDir(), "nested", "cache")

		st, err := NewStore(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer st.Close()

		if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
			t.Error("expected cache directory to be created")
		}
	})

	t.Run("fails with invalid directory", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("skipping test when running as root")
		}

		tmpFile := filepath.Join(t.TempDir(), "blocking-file")
		if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create blocking file: %v", err)
		}

		invalidDir := filepath.Join(tmpFile, "cache")
		_, err := NewStore(invalidDir)
		if err == nil {
			t.Error("expected error for invalid directory")
		}
	})
}

func TestStore_NewHTTPClient(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	client := st.NewHTTPClient(Options{})

	if client.Timeout != 10*time.Minute {
		t.Errorf("expected timeout 10m, got %v", client.Timeout)
	}

	if client.Transport == nil {
		t.Error("expected transport to be set")
	}

	t.Run("redirect limit", func(t *testing.T) {
		req := &http.Request{}
		via := make([]*http.Request, 5)

		err := client.CheckRedirect(req, via)
		if err == nil {
			t.Error("expected error for too many redirects")
		}
	})
}

func TestStore_NewReader(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	opt := Options{TTL: time.Hour, Retention: 24 * time.Hour, Compression: true}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:10.0,\ntest.ts\n"))
	}))
	defer server.Close()

	reader, err := st.newReader(ctx, server.URL, opt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer func() { _ = reader.Close() }()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read content: %v", err)
	}

	expected := "#EXTM3U\n#EXTINF:10.0,\ntest.ts\n"
	if string(content) != expected {
		t.Errorf("expected content %q, got %q", expected, string(content))
	}
}

func TestStore_CleanExpired(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	validName := "valid"
	validFile := filepath.Join(tmpDir, validName+compressedExtension)
	validMeta := filepath.Join(tmpDir, validName+metaExtension)
	if err := os.WriteFile(validFile, []byte("valid content"), 0644); err != nil {
		t.Fatalf("failed to create valid file: %v", err)
	}
	if err := createTestMetadata(validMeta, time.Now().Unix(), int64((24*time.Hour)/time.Second)); err != nil {
		t.Fatalf("failed to create valid metadata: %v", err)
	}

	expiredName := "expired"
	expiredFile := filepath.Join(tmpDir, expiredName+compressedExtension)
	expiredMeta := filepath.Join(tmpDir, expiredName+metaExtension)
	if err := os.WriteFile(expiredFile, []byte("expired content"), 0644); err != nil {
		t.Fatalf("failed to create expired file: %v", err)
	}
	if err := createTestMetadata(expiredMeta, time.Now().Add(-48*time.Hour).Unix(), int64((24*time.Hour)/time.Second)); err != nil {
		t.Fatalf("failed to create expired metadata: %v", err)
	}

	orphanedName := "orphaned"
	orphanedFile := filepath.Join(tmpDir, orphanedName+compressedExtension)
	if err := os.WriteFile(orphanedFile, []byte("orphaned content"), 0644); err != nil {
		t.Fatalf("failed to create orphaned file: %v", err)
	}

	if err := st.cleanExpired(); err != nil {
		t.Fatalf("failed to clean cache: %v", err)
	}

	checkFileExists(t, validFile, true)
	checkFileExists(t, validMeta, true)
	checkFileExists(t, expiredFile, false)
	checkFileExists(t, expiredMeta, false)
	checkFileExists(t, orphanedFile, false)
}

func TestStore_RemoveEntry(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	name := "test"
	dataPath := filepath.Join(tmpDir, name+compressedExtension)
	metaPath := filepath.Join(tmpDir, name+metaExtension)

	if err := os.WriteFile(dataPath, []byte("test data"), 0644); err != nil {
		t.Fatalf("failed to create data file: %v", err)
	}
	if err := createTestMetadata(metaPath, time.Now().Unix(), int64((24*time.Hour)/time.Second)); err != nil {
		t.Fatalf("failed to create metadata: %v", err)
	}

	if err := st.removeEntry(name); err != nil {
		t.Fatalf("failed to remove entry: %v", err)
	}

	checkFileExists(t, dataPath, false)
	checkFileExists(t, metaPath, false)
}

func createTestMetadata(path string, cachedAt int64, retention int64) error {
	metadata := Metadata{
		CachedAt: cachedAt,
		Headers:  make(map[string]string, len(forwardedHeaders)),
	}
	ret := retention
	metadata.RetentionSeconds = &ret

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	return json.NewEncoder(file).Encode(metadata)
}

func checkFileExists(t *testing.T, path string, shouldExist bool) {
	t.Helper()
	_, err := os.Stat(path)
	if shouldExist && os.IsNotExist(err) {
		t.Errorf("file %s should exist but doesn't", path)
	} else if !shouldExist && !os.IsNotExist(err) {
		t.Errorf("file %s should not exist but does", path)
	}
}
