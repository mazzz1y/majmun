package httpclient

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"majmun/internal/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

type status int

const (
	statusValid status = iota
	statusRenewed
	statusExpired
	statusNotFound
)

type Metadata struct {
	CachedAt         int64             `json:"cached_at"`
	RetentionSeconds *int64            `json:"retention_seconds"`
	Headers          map[string]string `json:"headers"`
}

type Reader struct {
	URL             string
	Name            string
	FilePath        string
	TmpFilePath     string
	MetaPath        string
	TmpMetaPath     string
	ReadCloser      io.ReadCloser
	cacheWrite      bool
	file            *os.File
	gzipWriter      *gzip.Writer
	originResponse  *http.Response
	client          *http.Client
	contentLength   int64
	downloadedBytes int64
	contentType     string
	ttl             time.Duration
	retention       time.Duration
	compression     bool
	eofReached      bool
}

func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.ReadCloser.Read(p)
	if err == io.EOF {
		r.eofReached = true
	}
	return
}

func (r *Reader) Close() error {
	var closers []func() error

	if r.originResponse != nil {
		closers = append(closers, r.originResponse.Body.Close)
	}
	if r.ReadCloser != nil {
		closers = append(closers, r.ReadCloser.Close)
	}
	if r.gzipWriter != nil {
		closers = append(closers, r.gzipWriter.Close)
	}
	if r.file != nil {
		closers = append(closers, r.file.Close)
	}

	var firstErr error
	for _, closer := range closers {
		if err := closer(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}

	if !r.cacheWrite {
		return nil
	}

	if !r.isDownloadComplete() {
		r.Cleanup()
		return nil
	}

	if r.TmpFilePath != "" {
		_ = os.Remove(r.FilePath)
		if err := os.Rename(r.TmpFilePath, r.FilePath); err != nil {
			r.Cleanup()
			return err
		}
	}

	if err := r.SaveMetadata(); err != nil {
		r.Cleanup()
		return err
	}

	return nil
}

func (r *Reader) getCachedHeaders() map[string]string {
	meta, err := readMetadata(r.MetaPath)
	if err != nil {
		return nil
	}
	return meta.Headers
}

func (r *Reader) createCacheFile() error {
	if r.TmpFilePath == "" {
		r.TmpFilePath = r.FilePath + ".tmp"
	}
	_ = os.Remove(r.TmpFilePath)
	file, err := os.Create(r.TmpFilePath)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	r.cacheWrite = true
	r.file = file
	return nil
}

func (r *Reader) isGzippedContent(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return contentType == "application/gzip" ||
		contentType == "application/x-gzip" ||
		resp.Header.Get("Content-Encoding") == "gzip" ||
		strings.HasSuffix(r.URL, ".gz")
}

func (r *Reader) isDownloadComplete() bool {
	if r.contentLength <= 0 {
		return r.eofReached
	}
	return r.downloadedBytes == r.contentLength && r.eofReached
}

func (r *Reader) checkCacheStatus() status {
	if _, err := os.Stat(r.MetaPath); err != nil {
		return statusNotFound
	}
	if _, err := os.Stat(r.FilePath); err != nil {
		return statusNotFound
	}

	meta, err := readMetadata(r.MetaPath)
	if err != nil {
		return statusNotFound
	}

	if r.ttl > 0 && time.Since(time.Unix(meta.CachedAt, 0)) < r.ttl {
		return statusValid
	}

	if exp, ok := meta.Headers["Expires"]; ok {
		if expires, err := time.Parse(time.RFC1123, exp); err == nil {
			if expires.After(time.Now()) {
				_ = r.SaveMetadata()
				return statusRenewed
			}
		}
	}

	return r.tryRenewal(&meta)
}

func (r *Reader) tryRenewal(meta *Metadata) status {
	var lastModified time.Time
	var etag string

	if lm := meta.Headers["Last-Modified"]; lm != "" {
		if parsedTime, err := time.Parse(time.RFC1123, lm); err == nil {
			lastModified = parsedTime
		}
	}

	etag = meta.Headers["Etag"]

	if !r.isModifiedSince(lastModified, etag) {
		if err := r.SaveMetadata(); err != nil {
			return statusExpired
		}
		return statusRenewed
	}

	return statusExpired
}

func (r *Reader) isModifiedSince(lastModified time.Time, etag string) bool {
	if lastModified.IsZero() && etag == "" {
		return true
	}

	req, err := http.NewRequest("HEAD", r.URL, nil)
	if err != nil {
		return true
	}

	if !lastModified.IsZero() {
		req.Header.Set("If-Modified-Since", lastModified.Format(time.RFC1123))
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return true
	}
	defer func() { _ = resp.Body.Close() }()

	r.originResponse = resp

	if resp.StatusCode == http.StatusNotModified {
		return false
	}

	if resp.StatusCode == http.StatusOK && !lastModified.IsZero() {
		if serverLastModified := resp.Header.Get("Last-Modified"); serverLastModified != "" {
			if serverTime, err := time.Parse(time.RFC1123, serverLastModified); err == nil {
				return serverTime.After(lastModified)
			}
		}
	}

	return true
}

func (r *Reader) newCachedReader() (io.ReadCloser, error) {
	file, err := os.Open(r.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cached file: %w", err)
	}

	r.file = file

	if r.compression {
		gzipR, err := gzip.NewReader(file)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return ioutil.NewReaderWithCloser(gzipR, gzipR.Close), nil
	} else {
		return file, nil
	}
}

func (r *Reader) newDirectReader(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	r.originResponse = resp
	r.contentType = resp.Header.Get("Content-Type")

	if r.isGzippedContent(resp) {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return ioutil.NewReaderWithCloser(gzipReader, gzipReader.Close), nil
	} else {
		return resp.Body, nil
	}
}

func (r *Reader) newCachingReader(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	r.originResponse = resp
	r.contentType = resp.Header.Get("Content-Type")
	r.contentLength = resp.ContentLength

	err = r.createCacheFile()
	if err != nil {
		_ = resp.Body.Close()
		return nil, err
	}

	var reader io.ReadCloser
	countReader := ioutil.NewCountReadCloser(resp.Body, &r.downloadedBytes)

	if r.compression {
		if r.isGzippedContent(resp) {
			tee := io.TeeReader(countReader, r.file)
			gzipReader, err := gzip.NewReader(tee)
			if err != nil {
				_ = r.file.Close()
				_ = countReader.Close()
				return nil, fmt.Errorf("failed to create gzip reader: %w", err)
			}
			reader = ioutil.NewReaderWithCloser(gzipReader, gzipReader.Close)
		} else {
			gzipW, err := gzip.NewWriterLevel(r.file, gzip.BestSpeed)
			if err != nil {
				_ = r.file.Close()
				_ = countReader.Close()
				return nil, fmt.Errorf("failed to create gzip writer: %w", err)
			}
			r.gzipWriter = gzipW
			reader = ioutil.NewReaderWithCloser(io.TeeReader(countReader, gzipW), gzipW.Close)
		}
	} else {
		if r.isGzippedContent(resp) {
			gzipReader, err := gzip.NewReader(countReader)
			if err != nil {
				_ = r.file.Close()
				_ = countReader.Close()
				return nil, fmt.Errorf("failed to create gzip reader: %w", err)
			}
			reader = ioutil.NewReaderWithCloser(io.TeeReader(gzipReader, r.file), gzipReader.Close)
		} else {
			reader = ioutil.NewReaderWithCloser(io.TeeReader(countReader, r.file), countReader.Close)
		}
	}

	return reader, nil
}
