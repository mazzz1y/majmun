package listing

import (
	"context"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

func GenerateHashID(parts ...string) string {
	h := fnv.New32a()
	for _, p := range parts {
		h.Write([]byte(p))
	}
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}

func CreateReader(ctx context.Context, httpClient HTTPClient, resourceURL string) (io.ReadCloser, error) {
	if isURL(resourceURL) {
		req, err := http.NewRequestWithContext(ctx, "GET", resourceURL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		return resp.Body, nil
	}

	reader, err := openLocalFile(resourceURL)
	if err != nil {
		return nil, err
	}

	return reader, nil
}

func isURL(path string) bool {
	u, err := url.Parse(path)
	if err != nil {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https"
}

func openLocalFile(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return file, nil
}
