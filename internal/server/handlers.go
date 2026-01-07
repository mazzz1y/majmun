package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"majmun/internal/app"
	"net/http"
	"time"

	"majmun/internal/ctxutil"
	"majmun/internal/listing/m3u8"
	"majmun/internal/listing/xmltv"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"majmun/internal/urlgen"
)

const streamContentType = "video/mp2t"

type responseHeaders map[string]string

var (
	playlistHeaders = responseHeaders{
		"Content-Type":  "application/x-mpegurl",
		"Cache-Control": "no-cache",
	}
	epgHeaders = responseHeaders{
		"Content-Type":  "application/xml",
		"Cache-Control": "no-cache",
	}
	epgGzipHeaders = responseHeaders{
		"Content-Type":        "application/gzip",
		"Cache-Control":       "no-cache",
		"Content-Disposition": `attachment; filename="epg.xml.gz"`,
	}
)

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	ctx := ctxutil.WithRequestType(r.Context(), metrics.RequestTypePlaylist)
	client := ctxutil.Client(ctx).(*app.Client)

	logging.Debug(ctx, "playlist request")

	setHeaders(w, playlistHeaders)

	streamer := m3u8.NewStreamer(
		client.PlaylistProviders(),
		client.EPGLink(),
		client.ChannelProcessor(),
		client.PlaylistProcessor(),
	)

	count, err := streamer.WriteTo(ctx, w)
	if err != nil {
		logging.Error(ctx, err, "failed to write playlist")
		if count == 0 {
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		}
		return
	}

	metrics.IncListingDownload(ctx)
}

func (s *Server) handleEPG(w http.ResponseWriter, r *http.Request) {
	ctx := ctxutil.WithRequestType(r.Context(), metrics.RequestTypeEPG)

	logging.Info(ctx, "epg request")

	streamer, err := s.prepareEPGStreamer(ctx)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	setHeaders(w, epgHeaders)

	count, err := streamer.WriteTo(ctx, w)
	if err != nil {
		logging.Error(ctx, err, "failed to write EPG")
		if count == 0 {
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		}
		return
	}

	metrics.IncListingDownload(ctx)
}

func (s *Server) handleEPGgz(w http.ResponseWriter, r *http.Request) {
	ctx := ctxutil.WithRequestType(r.Context(), metrics.RequestTypeEPG)

	logging.Debug(ctx, "gzipped epg request")

	streamer, err := s.prepareEPGStreamer(ctx)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	setHeaders(w, epgGzipHeaders)

	count, err := streamer.WriteToGzip(ctx, w)
	if err != nil {
		logging.Error(ctx, err, "failed to write gzipped epg")
		if count == 0 {
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		}
		return
	}

	metrics.IncListingDownload(ctx)
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := ctxutil.StreamData(ctx).(*urlgen.Data)

	logging.Debug(ctx, "handling proxy request", "type", data.RequestType)

	switch data.RequestType {
	case urlgen.RequestTypeFile:
		s.handleFileProxy(ctx, w, data)
	case urlgen.RequestTypeStream:
		s.handleStreamProxy(ctx, w, r)
	default:
		logging.Error(ctx, nil, "invalid proxy request type", "type", data.RequestType)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}
}

func (s *Server) handleFileProxy(ctx context.Context, w http.ResponseWriter, data *urlgen.Data) {
	ctx = ctxutil.WithRequestType(ctx, metrics.RequestTypeFile)

	stream := data.File

	logging.Debug(ctx, "proxying file", "url", stream.URL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stream.URL, nil)
	if err != nil {
		logging.Error(ctx, err, "failed to create request")
		http.Error(w, "Failed to create request", http.StatusBadGateway)
		return
	}

	resp, err := ctxutil.Provider(ctx).(app.Provider).HTTPClient().Do(req)
	if err != nil {
		logging.Error(ctx, err, "file proxy failed")
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode > 299 {
		logging.Error(ctx, fmt.Errorf("status code: %d", resp.StatusCode), "upstream returned error")
		http.Error(w, http.StatusText(resp.StatusCode), resp.StatusCode)
		return
	}

	for header, values := range resp.Header {
		w.Header()[header] = values
	}

	if _, err = io.Copy(w, resp.Body); err != nil {
		logging.Error(ctx, err, "file copy failed")
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(http.StatusText(http.StatusOK)))
}

func (s *Server) prepareEPGStreamer(ctx context.Context) (*xmltv.Streamer, error) {
	client := ctxutil.Client(ctx).(*app.Client)

	m3u8Streamer := m3u8.NewStreamer(
		client.PlaylistProviders(),
		"",
		client.ChannelProcessor(),
		client.PlaylistProcessor(),
	)

	channels, err := m3u8Streamer.GetAllChannels(ctx)
	if err != nil {
		logging.Error(ctx, err, "failed to get channels")
		return nil, err
	}
	return xmltv.NewStreamer(client.EPGProviders(), channels), nil
}

func setHeaders(w http.ResponseWriter, headers responseHeaders) {
	for key, value := range headers {
		w.Header().Set(key, value)
	}
}

func buildStreamURL(baseURL, rawQuery string) string {
	if rawQuery != "" {
		return baseURL + "?" + rawQuery
	}
	return baseURL
}

func buildStreamKey(baseURL, rawQuery string) string {
	if rawQuery != "" {
		return generateHash(baseURL+"?"+rawQuery, time.Now().Unix())
	}
	return generateHash(baseURL)
}

func generateHash(parts ...any) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprint(h, part)
	}
	return hex.EncodeToString(h.Sum(nil)[:4])
}
