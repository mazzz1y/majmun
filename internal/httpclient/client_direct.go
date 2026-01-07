package httpclient

import (
	"fmt"
	"majmun/internal/config/common"
	"net/http"
	"time"
)

type headerTransport struct {
	base    http.RoundTripper
	headers []common.NameValue
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	for _, header := range t.headers {
		req2.Header.Set(header.Name, header.Value)
	}
	return t.base.RoundTrip(req2)
}

func NewDirectClient(extraHeaders []common.NameValue) *http.Client {
	return &http.Client{
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			headers: extraHeaders,
		},
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			for _, header := range extraHeaders {
				req.Header.Set(header.Name, header.Value)
			}
			return nil
		},
	}
}
