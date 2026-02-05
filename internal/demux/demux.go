package demux

import (
	"context"
	"errors"
	"io"
	"majmun/internal/ctxutil"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"majmun/internal/utils"
	"sync"
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
	pool          *WriterPool
	rootCtx       context.Context
	rootCtxCancel context.CancelFunc
	streamLocks   sync.Map
}

func NewDemuxer() *Demuxer {
	rootCtx, cancel := context.WithCancel(context.Background())
	return &Demuxer{
		pool:          NewWriterPool(),
		rootCtx:       rootCtx,
		rootCtxCancel: cancel,
	}
}

func (m *Demuxer) Stop() {
	if m.rootCtxCancel != nil {
		m.rootCtxCancel()
	}
	m.pool.Stop()
}

func (m *Demuxer) LockStream(streamKey string) func() {
	mutex, _ := m.streamLocks.LoadOrStore(streamKey, &sync.Mutex{})
	mtx := mutex.(*sync.Mutex)

	mtx.Lock()
	return mtx.Unlock
}

func (m *Demuxer) GetReader(ctx context.Context, req Request) (io.ReadCloser, error) {
	unlock := m.LockStream(req.StreamKey)
	defer unlock()

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

	isNewStream := m.pool.AddClient(req.StreamKey, pw)
	if isNewStream {
		if utils.AcquireSemaphore(streamCtx, req.Semaphore, semaphoreTimeout, "subscription") {
			logging.Debug(streamCtx, "acquired subscription semaphore")
		} else {
			_ = sr.Close()
			return nil, ErrSubscriptionSemaphore
		}
		go m.startStream(streamCtx, req)
		logging.Info(streamCtx, "started new stream")
	} else {
		metrics.IncStreamsReused(ctx)
		logging.Info(clientCtx, "joined existing stream")
	}

	return sr, nil
}

func (m *Demuxer) startStream(ctx context.Context, req Request) {
	key := req.StreamKey

	metrics.IncPlaylistStreamsActive(ctx)
	defer metrics.DecPlaylistStreamsActive(ctx)

	unlock := m.LockStream(key)
	writer := m.pool.GetWriter(key)
	unlock()

	if writer == nil {
		logging.Error(ctx, errors.New("writer is nil"), "failed to get writer")
		return
	}

	streamID := ctxutil.StreamID(ctx)
	streamCtx, cancel := context.WithCancel(ctxutil.WithStreamID(context.Background(), streamID))
	defer cancel()
	defer writer.Close()

	go func() {
		emptyCh := writer.IsEmptyChannel()
		defer writer.CancelEmptyChannel(emptyCh)

		if req.Semaphore != nil {
			defer req.Semaphore.Release(1)
			defer logging.Debug(ctx, "releasing subscription semaphore")
		}

		select {
		case <-emptyCh:
			logging.Debug(ctx, "no clients left, stopping stream")
			cancel()
			return
		case <-streamCtx.Done():
			logging.Debug(ctx, "context canceled, stopping stream")
			return
		}
	}()

	_, err := req.Streamer.Stream(streamCtx, writer)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logging.Debug(streamCtx, "stream canceled")
		} else {
			logging.Error(streamCtx, err, "stream failed")
		}
		logging.Debug(streamCtx, "stream ended")
	}
}
