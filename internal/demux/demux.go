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

func (d *Demuxer) Stop() {
	d.pool.Stop()
}

func (d *Demuxer) GetReader(ctx context.Context, req Request) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	clientCtx := ctxutil.WithStreamID(ctx, req.StreamKey)
	streamCtx := context.WithoutCancel(clientCtx)

	sr := &streamReader{PipeReader: pr}

	stop := context.AfterFunc(clientCtx, func() {
		_ = pr.CloseWithError(clientCtx.Err())
		_ = sr.Close()
	})

	sr.closeFunc = func() {
		stop()
		_ = pw.Close()
		d.pool.RemoveClient(req.StreamKey, pw)
		logging.Debug(clientCtx, "reader closed")
	}

	writer, isNew := d.pool.AddClient(req.StreamKey, pw)
	if isNew {
		if !utils.AcquireSemaphore(streamCtx, req.Semaphore, semaphoreTimeout, "subscription") {
			d.pool.CloseStream(req.StreamKey)
			_ = sr.Close()
			return nil, ErrSubscriptionSemaphore
		}
		logging.Debug(streamCtx, "acquired subscription semaphore")
		go d.startStream(streamCtx, req, writer)
		logging.Info(streamCtx, "started new stream")
	} else {
		metrics.IncStreamsReused(ctx)
		logging.Info(clientCtx, "joined existing stream")
	}

	return sr, nil
}

func (d *Demuxer) startStream(ctx context.Context, req Request, writer *StreamWriter) {
	metrics.IncPlaylistStreamsActive(ctx)
	defer metrics.DecPlaylistStreamsActive(ctx)
	defer d.pool.CloseStream(req.StreamKey)
	defer func() {
		if req.Semaphore != nil {
			req.Semaphore.Release(1)
			logging.Debug(ctx, "releasing subscription semaphore")
		}
	}()

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-writer.WaitEmpty():
			logging.Debug(ctx, "no clients left, stopping stream")
			cancel()
		case <-streamCtx.Done():
		}
	}()

	_, err := req.Streamer.Stream(streamCtx, writer)
	if err != nil && !errors.Is(err, context.Canceled) {
		logging.Error(ctx, err, "stream failed")
	}
}
