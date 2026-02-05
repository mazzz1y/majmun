package demux

import (
	"context"
	"errors"
	"io"
	"majmun/internal/ctxutil"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"majmun/internal/utils"
	"time"

	"golang.org/x/sync/semaphore"
)

const semaphoreTimeout = 200 * time.Millisecond

var (
	ErrSubscriptionSemaphore = errors.New("failed to acquire subscription semaphore")
)

type Streamer interface {
	Stream(ctx context.Context, w io.Writer) (int64, error)
}

type Request struct {
	Context   context.Context
	StreamKey string
	Streamer  Streamer
	Semaphore *semaphore.Weighted
}

type Demuxer struct {
	pool *WriterPool
}

func NewDemuxer() *Demuxer {
	return &Demuxer{
		pool: NewWriterPool(),
	}
}

func (m *Demuxer) Stop() {
	m.pool.Stop()
}

func (m *Demuxer) GetReader(ctx context.Context, req Request) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	clientCtx := ctxutil.WithStreamID(ctx, req.StreamKey)
	streamCtx := context.WithoutCancel(clientCtx)

	sr := &streamReader{
		PipeReader: pr,
	}

	stop := context.AfterFunc(clientCtx, func() {
		_ = pr.CloseWithError(clientCtx.Err())
		_ = sr.Close()
	})

	sr.closeFunc = func() {
		stop()
		_ = pw.Close()
		m.pool.RemoveClient(req.StreamKey, pw)
		logging.Debug(clientCtx, "reader closed")
	}

	writer, isNewStream := m.pool.AddClient(req.StreamKey, pw)
	if isNewStream {
		if !utils.AcquireSemaphore(streamCtx, req.Semaphore, semaphoreTimeout, "subscription") {
			m.pool.RemoveAndClose(req.StreamKey)
			_ = sr.Close()
			return nil, ErrSubscriptionSemaphore
		}
		logging.Debug(streamCtx, "acquired subscription semaphore")
		go m.startStream(streamCtx, req, writer)
		logging.Info(streamCtx, "started new stream")
	} else {
		metrics.IncStreamsReused(ctx)
		logging.Info(clientCtx, "joined existing stream")
	}

	return sr, nil
}

func (m *Demuxer) startStream(ctx context.Context, req Request, writer *StreamWriter) {
	metrics.IncPlaylistStreamsActive(ctx)
	defer metrics.DecPlaylistStreamsActive(ctx)
	defer m.pool.RemoveAndClose(req.StreamKey)

	streamID := ctxutil.StreamID(ctx)
	streamCtx, cancel := context.WithCancel(ctxutil.WithStreamID(context.Background(), streamID))
	defer cancel()

	go func() {
		if req.Semaphore != nil {
			defer req.Semaphore.Release(1)
			defer logging.Debug(ctx, "releasing subscription semaphore")
		}

		select {
		case <-writer.EmptyChannel():
			logging.Debug(ctx, "no clients left, stopping stream")
			cancel()
		case <-streamCtx.Done():
			logging.Debug(ctx, "context canceled, stopping stream")
		}
	}()

	_, err := req.Streamer.Stream(streamCtx, writer)
	if err != nil && !errors.Is(err, context.Canceled) {
		logging.Error(streamCtx, err, "stream failed")
	}
}
