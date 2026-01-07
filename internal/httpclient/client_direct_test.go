package httpclient

import (
	"fmt"
	"majmun/internal/config/common"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewDirectHTTPClient_ExtraHeaders(t *testing.T) {
	extraHeaders := []common.NameValue{
		{Name: "X-Custom-Header", Value: "custom-value"},
		{Name: "Authorization", Value: "Bearer token123"},
	}

	var requestCount int
	var capturedHeaders []http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = append(capturedHeaders, r.Header.Clone())
		requestCount++

		if requestCount < 3 {
			redirectURL := fmt.Sprintf("http://%s/redirect%d", r.Host, requestCount)
			w.Header().Set("Location", redirectURL)
			w.WriteHeader(http.StatusFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := NewDirectClient(extraHeaders)
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if requestCount != 3 {
		t.Errorf("expected 3 requests (1 initial + 2 redirects), got %d", requestCount)
	}

	for i, headers := range capturedHeaders {
		for _, header := range extraHeaders {
			if got := headers.Get(header.Name); got != header.Value {
				t.Errorf("request %d: header %s = %q, want %q", i+1, header.Name, got, header.Value)
			}
		}
	}
}

func TestNewDirectHTTPClient_TooManyRedirects(t *testing.T) {
	extraHeaders := []common.NameValue{
		{Name: "X-Test", Value: "value"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectURL := fmt.Sprintf("http://%s/redirect", r.Host)
		w.Header().Set("Location", redirectURL)
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	client := NewDirectClient(extraHeaders)
	_, err := client.Get(server.URL)

	if err == nil {
		t.Fatal("expected error for too many redirects, got nil")
	}
}

func TestNewDirectHTTPClient_NoRedirects(t *testing.T) {
	extraHeaders := []common.NameValue{
		{Name: "X-API-Key", Value: "secret123"},
	}

	var capturedHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewDirectClient(extraHeaders)
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	for _, header := range extraHeaders {
		if got := capturedHeader.Get(header.Name); got != header.Value {
			t.Errorf("header %s = %q, want %q", header.Name, got, header.Value)
		}
	}
}

func TestNewDirectHTTPClient_Transport(t *testing.T) {
	extraHeaders := []common.NameValue{
		{Name: "X-Test", Value: "value"},
		{Name: "X-Another", Value: "another-value"},
	}

	client := NewDirectClient(extraHeaders)

	transport, ok := client.Transport.(*headerTransport)
	if !ok {
		t.Fatalf("expected *headerTransport, got %T", client.Transport)
	}

	if transport.base != http.DefaultTransport {
		t.Errorf("expected default transport to be wrapped, got %v", transport.base)
	}

	if len(transport.headers) != len(extraHeaders) {
		t.Errorf("expected %d headers, got %d", len(extraHeaders), len(transport.headers))
	}

	for _, expected := range extraHeaders {
		found := false
		for _, actual := range transport.headers {
			if actual.Name == expected.Name && actual.Value == expected.Value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("header %s:%s not found in transport", expected.Name, expected.Value)
		}
	}
}

func TestNewDirectHTTPClient_EmptyHeaders(t *testing.T) {
	client := NewDirectClient(nil)

	transport, ok := client.Transport.(*headerTransport)
	if !ok {
		t.Errorf("expected *headerTransport, got %T", client.Transport)
	} else if len(transport.headers) != 0 {
		t.Error("expected empty headers")
	}
}
