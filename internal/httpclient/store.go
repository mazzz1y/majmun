package httpclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	compressedExtension   = ".gz"
	uncompressedExtension = ".cache"
	metaExtension         = ".meta"
)

type Store struct {
	dir           string
	cleanupTicker *time.Ticker
	doneCh        chan struct{}
}

func NewStore(path string) (*Store, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	s := &Store{
		dir:           path,
		cleanupTicker: time.NewTicker(24 * time.Hour),
		doneCh:        make(chan struct{}),
	}

	go s.cleanupRoutine()

	return s, nil
}

func (s *Store) NewHTTPClient(opt Options) *http.Client {
	return &http.Client{
		Transport: &cachingTransport{store: s, opt: opt},
		Timeout:   10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func (s *Store) Close() {
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}
	if s.doneCh != nil {
		close(s.doneCh)
	}
}

func (s *Store) cleanupRoutine() {
	for {
		select {
		case <-s.cleanupTicker.C:
			if err := s.cleanExpired(); err != nil {
				logging.Error(context.Background(), err, "failed to clean expired cache")
			}
		case <-s.doneCh:
			return
		}
	}
}

func (s *Store) cleanExpired() error {
	allFiles, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("failed to list cache directory: %w", err)
	}

	expiredRemoved := 0
	orphanedRemoved := 0
	now := time.Now()

	for _, file := range allFiles {
		fileName := file.Name()
		filePath := filepath.Join(s.dir, fileName)

		dataExt := ""
		switch {
		case strings.HasSuffix(fileName, compressedExtension):
			dataExt = compressedExtension
		case strings.HasSuffix(fileName, uncompressedExtension):
			dataExt = uncompressedExtension
		}
		if dataExt != "" {
			name := strings.TrimSuffix(fileName, dataExt)
			metaPath := filepath.Join(s.dir, name+metaExtension)

			_, err := os.Stat(metaPath)
			if os.IsNotExist(err) {
				if err := os.Remove(filePath); err != nil {
					return fmt.Errorf("failed to remove orphaned file: %w", err)
				}
				orphanedRemoved++
			}
			continue
		}

		switch {
		case strings.HasSuffix(fileName, metaExtension):
			metaPath := filepath.Join(s.dir, fileName)
			metadata, err := readMetadata(metaPath)
			if err != nil {
				if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove invalid meta file: %w", err)
				}
				orphanedRemoved++
				continue
			}
			if metadata.RetentionSeconds == nil {
				if now.Sub(time.Unix(metadata.CachedAt, 0)) > 24*time.Hour {
					name := strings.TrimSuffix(fileName, metaExtension)
					if err := s.removeEntry(name); err != nil {
						return err
					}
					expiredRemoved++
				}
				continue
			}

			retention := time.Duration(*metadata.RetentionSeconds) * time.Second
			if retention <= 0 {
				name := strings.TrimSuffix(fileName, metaExtension)
				if err := s.removeEntry(name); err != nil {
					return err
				}
				expiredRemoved++
				continue
			}

			if now.Sub(time.Unix(metadata.CachedAt, 0)) > retention {
				name := strings.TrimSuffix(fileName, metaExtension)
				if err := s.removeEntry(name); err != nil {
					return err
				}
				expiredRemoved++
			}

		default:
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove unexpected file: %w", err)
			}
			orphanedRemoved++
		}
	}

	args := []any{"total_files", len(allFiles), "expired", expiredRemoved, "orphaned", orphanedRemoved}
	logging.Info(context.Background(), "cache cleanup", args...)
	return nil
}

func (s *Store) removeEntry(name string) error {
	gzPath := filepath.Join(s.dir, name+compressedExtension)
	normalPath := filepath.Join(s.dir, name+uncompressedExtension)
	metaPath := filepath.Join(s.dir, name+metaExtension)

	gzErr := os.Remove(gzPath)
	if gzErr != nil && !os.IsNotExist(gzErr) {
		return gzErr
	}

	normalErr := os.Remove(normalPath)
	if normalErr != nil && !os.IsNotExist(normalErr) {
		return normalErr
	}

	metaErr := os.Remove(metaPath)
	if metaErr != nil && !os.IsNotExist(metaErr) {
		return metaErr
	}

	return nil
}

func (s *Store) newReader(ctx context.Context, url string, opt Options) (*Reader, error) {
	h := sha256.New()
	_, _ = io.WriteString(h, url)
	_, _ = io.WriteString(h, "\n")
	_, _ = io.WriteString(h, opt.key())
	name := hex.EncodeToString(h.Sum(nil)[:16])

	fileExt := uncompressedExtension
	if opt.Compression {
		fileExt = compressedExtension
	}

	reader := &Reader{
		URL:         url,
		Name:        name,
		FilePath:    filepath.Join(s.dir, name+fileExt),
		TmpFilePath: filepath.Join(s.dir, name+fileExt+".tmp"),
		MetaPath:    filepath.Join(s.dir, name+metaExtension),
		TmpMetaPath: filepath.Join(s.dir, name+metaExtension+".tmp"),
		client:      NewDirectClient(opt.HTTPHeaders),
		ttl:         opt.TTL,
		compression: opt.Compression,
		retention:   opt.Retention,
	}

	var err error
	var readCloser io.ReadCloser
	cacheStatus := metrics.CacheStatusMiss

	st := reader.checkCacheStatus()

	switch st {
	case statusValid, statusRenewed:
		readCloser, err = reader.newCachedReader()
		if err == nil {
			if st == statusValid {
				cacheStatus = metrics.CacheStatusHit
			} else {
				cacheStatus = metrics.CacheStatusRenewed
			}
			reader.ReadCloser = readCloser
		}
	default:
		cacheStatus = metrics.CacheStatusMiss
		readCloser, err = reader.newCachingReader(ctx)
		if err == nil {
			reader.ReadCloser = readCloser
		}
	}

	logging.Debug(ctx, "file access", "cache", cacheStatus, "url", logging.SanitizeURL(url))
	metrics.IncProxyRequests(ctx, cacheStatus)

	return reader, err
}
