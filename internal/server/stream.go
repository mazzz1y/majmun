package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"syscall"
	"time"

	"majmun/internal/app"
	"majmun/internal/ctxutil"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"majmun/internal/streampool"
	"majmun/internal/urlgen"
)

const (
	maxRetryAttempts = 2
	retryTimeout     = 3 * time.Second
)

type streamResult struct {
	success         bool
	isLimitError    bool
	isUpstreamError bool
}

type allStreamsResult struct {
	success          bool
	hasLimitError    bool
	hasUpstreamError bool
	defaultProvider  *app.Playlist
}

func (s *Server) handleStreamProxy(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	logging.Debug(ctx, "proxying stream")

	client := ctxutil.Client(ctx).(*app.Client)
	data := ctxutil.StreamData(ctx).(*urlgen.Data)

	ctx = ctxutil.WithRequestType(ctx, metrics.RequestTypePlaylist)
	ctx = ctxutil.WithChannelName(ctx, data.StreamData.ChannelName)

	if !s.acquireSemaphores(ctx) {
		logging.Error(ctx, errors.New("failed to acquire semaphores"), "stream proxy failed")
		if len(data.StreamData.Streams) > 0 {
			firstProvider := client.GetProvider(
				data.StreamData.Streams[0].ProviderInfo.ProviderType,
				data.StreamData.Streams[0].ProviderInfo.ProviderName,
			)
			if playlist, ok := firstProvider.(*app.Playlist); ok && playlist != nil {
				_, err := playlist.LimitStreamer().RunWithStdout(ctx, w)
				if err != nil {
					logging.Error(ctx, err, "failed to stream limit response")
				}
				return
			}
		}
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	defer s.releaseSemaphores(ctx)

	var lastResult allStreamsResult

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if attempt > 0 && lastResult.hasLimitError {
			logging.Debug(ctx, "sleeping before retry attempt")
			time.Sleep(retryTimeout)
		}

		result := s.tryAllStreams(ctx, w, r, client, data)
		if result.success {
			return
		}

		lastResult = result
	}

	if lastResult.hasLimitError && lastResult.defaultProvider != nil {
		_, err := lastResult.defaultProvider.LimitStreamer().RunWithStdout(ctx, w)
		if err != nil {
			logging.Error(ctx, err, "failed to stream limit response")
		}
		return
	}

	if lastResult.hasUpstreamError && lastResult.defaultProvider != nil {
		_, err := lastResult.defaultProvider.UpstreamErrorStreamer().RunWithStdout(ctx, w)
		if err != nil {
			logging.Error(ctx, err, "failed to stream upstream error response")
		}
		return
	}

	http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
}

func (s *Server) tryAllStreams(
	ctx context.Context, w http.ResponseWriter, r *http.Request, client *app.Client, data *urlgen.Data) allStreamsResult {
	var hasLimitError bool
	var hasUpstreamError bool
	var firstProvider *app.Playlist

	for i, stream := range data.StreamData.Streams {
		logging.Debug(ctx, "trying stream source", "index", i)

		provider := client.GetProvider(stream.ProviderInfo.ProviderType, stream.ProviderInfo.ProviderName)
		if provider == nil {
			logging.Error(ctx, errors.New("provider not found"), "stream_index", i,
				"provider_type", stream.ProviderInfo.ProviderType,
				"provider_name", stream.ProviderInfo.ProviderName)
			continue
		}

		playlist, ok := provider.(*app.Playlist)
		if !ok {
			logging.Error(ctx, errors.New("provider is not a playlist"), "stream_index", i)
			continue
		}

		if firstProvider == nil {
			firstProvider = playlist
		}

		result := s.tryStream(ctx, w, r, playlist, stream, i)

		if result.success {
			return allStreamsResult{true, false, false, nil}
		}

		if result.isLimitError {
			hasLimitError = true
		} else if result.isUpstreamError {
			hasUpstreamError = true
		}
	}

	return allStreamsResult{
		false, hasLimitError, hasUpstreamError, firstProvider}
}

func (s *Server) tryStream(
	ctx context.Context,
	w http.ResponseWriter, r *http.Request,
	playlist *app.Playlist, stream urlgen.Stream, streamIndex int) streamResult {

	ctx = ctxutil.WithChannelHidden(ctx, stream.Hidden)
	ctx = ctxutil.WithProviderType(ctx, metrics.RequestTypePlaylist)
	ctx = ctxutil.WithProviderName(ctx, playlist.Name())

	streamURL := buildStreamURL(stream.URL, r.URL.RawQuery)
	streamKey := buildStreamKey(stream.URL, r.URL.RawQuery)

	streamReq := streampool.Request{
		StreamKey:      streamKey,
		StreamURL:      streamURL,
		ClientStreamer: playlist.ClientStreamer,
		Semaphore:      playlist.Semaphore(),
		Segmenter:      playlist.SegmenterConfig(),
	}

	reader, err := s.streamPool.GetReader(ctx, streamReq)
	if errors.Is(err, streampool.ErrSubscriptionSemaphore) {
		s.handleSubscriptionError(ctx, streamIndex)
		return streamResult{false, true, false}
	}
	if err != nil {
		logging.Error(ctx, err, "failed to get stream", "stream_index", streamIndex)
		return streamResult{false, false, false}
	}
	defer func() { _ = reader.Close() }()

	logging.Debug(ctx, "started stream", "stream_index", streamIndex)
	return s.streamToResponse(ctx, w, reader)
}

func (s *Server) handleSubscriptionError(ctx context.Context, streamIndex int) {
	logging.Error(
		ctx, streampool.ErrSubscriptionSemaphore,
		"failed to get stream - subscription semaphore", "stream_index", streamIndex)

	metrics.IncStreamsFailures(ctx, metrics.FailureReasonPlaylistLimit)
}

func (s *Server) streamToResponse(
	ctx context.Context, w http.ResponseWriter, reader io.ReadCloser) streamResult {

	metrics.IncClientStreamsActive(ctx)
	defer metrics.DecClientStreamsActive(ctx)

	w.Header().Set("Content-Type", streamContentType)
	written, err := io.Copy(w, reader)

	if err == nil && written == 0 {
		logging.Error(ctx, errors.New("no data written to response"), "stream empty")
		metrics.IncStreamsFailures(ctx, metrics.FailureReasonUpstreamError)
		return streamResult{false, false, true}
	}

	if err != nil && !isClientDisconnect(err) {
		logging.Error(ctx, err, "error copying stream to response")
	}

	return streamResult{true, false, false}
}

func isClientDisconnect(err error) bool {
	return errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET)
}
