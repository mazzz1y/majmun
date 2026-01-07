package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReader_isModifiedSince(t *testing.T) {
	tests := []struct {
		name               string
		lastModified       time.Time
		etag               string
		serverLastModified string
		serverEtag         string
		serverStatus       int
		expectedResult     bool
		description        string
	}{
		{
			name:           "zero lastModified and empty etag returns true",
			lastModified:   time.Time{},
			etag:           "",
			expectedResult: true,
			description:    "when cached lastModified is zero and etag is empty, assume modified",
		},
		{
			name:           "server error returns true",
			lastModified:   time.Now().Add(-1 * time.Hour),
			etag:           `"abc123"`,
			serverStatus:   http.StatusInternalServerError,
			expectedResult: true,
			description:    "when server returns error, assume modified",
		},
		{
			name:               "resource modified by Last-Modified",
			lastModified:       time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			serverLastModified: "Mon, 01 Jan 2024 12:00:00 GMT",
			serverStatus:       http.StatusOK,
			expectedResult:     true,
			description:        "when server's Last-Modified is newer, return true",
		},
		{
			name:               "resource not modified by Last-Modified",
			lastModified:       time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			serverLastModified: "Sun, 01 Jan 2023 12:00:00 GMT",
			serverStatus:       http.StatusOK,
			expectedResult:     false,
			description:        "when server's Last-Modified is older, return false",
		},
		{
			name:           "resource not modified by ETag - 304 response",
			etag:           `"abc123"`,
			serverEtag:     `"abc123"`,
			serverStatus:   http.StatusNotModified,
			expectedResult: false,
			description:    "when server returns 304 Not Modified, return false",
		},
		{
			name:           "resource modified by ETag - 200 response",
			etag:           `"abc123"`,
			serverEtag:     `"def456"`,
			serverStatus:   http.StatusOK,
			expectedResult: true,
			description:    "when server returns 200 with different ETag, return true",
		},
		{
			name:               "both Last-Modified and ETag - not modified",
			lastModified:       time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			etag:               `"abc123"`,
			serverLastModified: "Sun, 01 Jan 2023 12:00:00 GMT",
			serverEtag:         `"abc123"`,
			serverStatus:       http.StatusNotModified,
			expectedResult:     false,
			description:        "when both headers match and server returns 304, return false",
		},
		{
			name:               "both Last-Modified and ETag - modified",
			lastModified:       time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			etag:               `"abc123"`,
			serverLastModified: "Mon, 01 Jan 2024 12:00:00 GMT",
			serverEtag:         `"def456"`,
			serverStatus:       http.StatusOK,
			expectedResult:     true,
			description:        "when either header indicates change, return true",
		},
		{
			name:           "no Last-Modified header returns true",
			lastModified:   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			serverStatus:   http.StatusOK,
			expectedResult: true,
			description:    "when server doesn't provide Last-Modified header, assume modified",
		},
		{
			name:               "invalid Last-Modified header returns true",
			lastModified:       time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			serverLastModified: "invalid-date-format",
			serverStatus:       http.StatusOK,
			expectedResult:     true,
			description:        "when server's Last-Modified is invalid, assume modified",
		},
		{
			name:           "ETag only - not modified",
			etag:           `"xyz789"`,
			serverEtag:     `"xyz789"`,
			serverStatus:   http.StatusNotModified,
			expectedResult: false,
			description:    "when only ETag is used and matches, return false",
		},
		{
			name:           "weak ETag support",
			etag:           `W/"weak-tag"`,
			serverEtag:     `W/"weak-tag"`,
			serverStatus:   http.StatusNotModified,
			expectedResult: false,
			description:    "when weak ETags match, return false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "HEAD", r.Method)

				if !tt.lastModified.IsZero() {
					expectedIfModifiedSince := tt.lastModified.Format(time.RFC1123)
					assert.Equal(t, expectedIfModifiedSince, r.Header.Get("If-Modified-Since"))
				}
				if tt.etag != "" {
					assert.Equal(t, tt.etag, r.Header.Get("If-None-Match"))
				}

				// Set response headers
				if tt.serverLastModified != "" {
					w.Header().Set("Last-Modified", tt.serverLastModified)
				}
				if tt.serverEtag != "" {
					w.Header().Set("ETag", tt.serverEtag)
				}

				if tt.serverStatus != 0 {
					w.WriteHeader(tt.serverStatus)
				}
			}))
			defer server.Close()

			reader := &Reader{
				URL:    server.URL,
				client: server.Client(),
			}

			result := reader.isModifiedSince(tt.lastModified, tt.etag)

			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

func TestReader_isModifiedSince_RequestFailed(t *testing.T) {
	reader := &Reader{
		URL:    "http://invalid-url-that-does-not-exist",
		client: &http.Client{Timeout: 1 * time.Millisecond},
	}

	lastModified := time.Now().Add(-1 * time.Hour)
	result := reader.isModifiedSince(lastModified, `"test-etag"`)

	assert.True(t, result, "should return true when HTTP request fails")
}

func TestReader_isModifiedSince_InvalidURL(t *testing.T) {
	reader := &Reader{
		URL:    "://invalid-url",
		client: &http.Client{},
	}

	lastModified := time.Now().Add(-1 * time.Hour)
	result := reader.isModifiedSince(lastModified, `"test-etag"`)

	assert.True(t, result, "should return true when URL is invalid")
}

func TestReader_tryRenewal(t *testing.T) {
	tests := []struct {
		name           string
		metadata       *Metadata
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedStatus status
		description    string
	}{
		{
			name: "renewal with Last-Modified - not modified",
			metadata: &Metadata{
				Headers: map[string]string{
					"Last-Modified": "Sun, 01 Jan 2023 12:00:00 GMT",
				},
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotModified)
			},
			expectedStatus: statusRenewed,
			description:    "should return statusRenewed when content not modified",
		},
		{
			name: "renewal with ETag - not modified",
			metadata: &Metadata{
				Headers: map[string]string{
					"Etag": `"abc123"`,
				},
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotModified)
			},
			expectedStatus: statusRenewed,
			description:    "should return statusRenewed when ETag matches",
		},
		{
			name: "renewal with both headers - modified",
			metadata: &Metadata{
				Headers: map[string]string{
					"Last-Modified": "Sun, 01 Jan 2023 12:00:00 GMT",
					"Etag":          `"abc123"`,
				},
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 12:00:00 GMT")
				w.Header().Set("ETag", `"def456"`)
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: statusExpired,
			description:    "should return statusExpired when content is modified",
		},
		{
			name: "renewal without validation headers",
			metadata: &Metadata{
				Headers: map[string]string{},
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: statusExpired,
			description:    "should return statusExpired when no validation headers available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			reader := &Reader{
				URL:         server.URL,
				MetaPath:    "/tmp/nonexistent.meta",
				TmpMetaPath: "/tmp/nonexistent.meta.tmp",
				client:      server.Client(),
			}

			result := reader.tryRenewal(tt.metadata)
			assert.Equal(t, tt.expectedStatus, result, tt.description)
		})
	}
}
