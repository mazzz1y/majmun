package streampool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"majmun/internal/config/proxy"
	"majmun/internal/ctxutil"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"majmun/internal/utils"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/semaphore"
)

const semaphoreTimeout = 200 * time.Millisecond

var (
	ErrSubscriptionSemaphore = errors.New("failed to acquire subscription semaphore")
	ErrSegmenterFailed       = errors.New("segmenter failed to start")
)

type Streamer interface {
	RunWithStdout(ctx context.Context, w io.Writer) (int64, error)
}

type Request struct {
	StreamKey string
	Streamer  Streamer
	Semaphore *semaphore.Weighted
	Segmenter proxy.Segmenter
}

type StreamPool struct {
	pool    *segmenterPool
	baseDir string
}

func New() *StreamPool {
	dir := filepath.Join(os.TempDir(), "majmun-segments")
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0755)

	return &StreamPool{
		pool:    newSegmenterPool(),
		baseDir: dir,
	}
}

func (d *StreamPool) Stop() {
	d.pool.stopAll()
	_ = os.RemoveAll(d.baseDir)
}

func (d *StreamPool) GetReader(ctx context.Context, req Request) (io.ReadCloser, error) {
	clientCtx := ctxutil.WithStreamID(ctx, req.StreamKey)
	streamCtx := context.WithoutCancel(clientCtx)

	seg, isNew, err := d.pool.getOrCreate(req.StreamKey, streamCtx, d.baseDir, req.Segmenter)
	if err != nil {
		return nil, err
	}

	if isNew {
		if !utils.AcquireSemaphore(streamCtx, req.Semaphore, semaphoreTimeout, "subscription") {
			d.pool.remove(req.StreamKey)
			return nil, ErrSubscriptionSemaphore
		}
		logging.Debug(streamCtx, "acquired subscription semaphore")
		go d.runSegmenter(streamCtx, req, seg)
		logging.Info(streamCtx, "started new segmenter")
	} else {
		seg.addClient()
		metrics.IncStreamsReused(ctx)
		logging.Info(clientCtx, "joined existing segmenter")
	}

	select {
	case <-seg.ready:
	case <-clientCtx.Done():
		seg.removeClient()
		return nil, clientCtx.Err()
	}

	if seg.startErr != nil {
		seg.removeClient()
		return nil, fmt.Errorf("%w: %v", ErrSegmenterFailed, seg.startErr)
	}

	cs, err := newClientStream(clientCtx, seg.playlistURL)
	if err != nil {
		seg.removeClient()
		return nil, fmt.Errorf("start client stream: %w", err)
	}

	return &clientReader{
		clientStream: cs,
		seg:          seg,
	}, nil
}

func (d *StreamPool) runSegmenter(ctx context.Context, req Request, seg *segmenter) {
	metrics.IncPlaylistStreamsActive(ctx)
	defer metrics.DecPlaylistStreamsActive(ctx)
	defer d.pool.remove(req.StreamKey)
	defer func() {
		if req.Semaphore != nil {
			req.Semaphore.Release(1)
			logging.Debug(ctx, "releasing subscription semaphore")
		}
	}()

	segCtx, segCancel := context.WithCancel(ctx)
	defer segCancel()

	go func() {
		select {
		case <-seg.waitEmpty():
			logging.Debug(ctx, "no clients left, stopping segmenter")
			segCancel()
		case <-segCtx.Done():
		}
	}()

	pr, pw := io.Pipe()

	go func() {
		defer func() { _ = pw.Close() }()
		_, err := req.Streamer.RunWithStdout(segCtx, pw)
		if err != nil && !errors.Is(err, context.Canceled) {
			logging.Error(ctx, err, "upstream stream failed")
		}
	}()

	seg.start(pr)
	_ = pr.Close()

	<-seg.waitEmpty()
}
