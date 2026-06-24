package streampool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"majmun/internal/config/common"
	"majmun/internal/ctxutil"
	"majmun/internal/hashid"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"majmun/internal/utils"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/semaphore"
)

// RunnerSpec is the resolved command and timing the stream pool needs to launch an ffmpeg
// process. Providers build it from their cascaded config, so the pool never sees unset values.
type RunnerSpec struct {
	Command      common.StringOrArr
	EnvVars      []common.NameValue
	TemplateVars []common.NameValue
	InitSegments int
	ReadyTimeout time.Duration
}

const semaphoreTimeout = 200 * time.Millisecond

var (
	ErrSubscriptionSemaphore = errors.New("failed to acquire subscription semaphore")
	ErrSegmenterFailed       = errors.New("segmenter failed to start")
)

type Streamer interface {
	RunWithStdout(ctx context.Context, w io.Writer) (int64, error)
}

type ClientStreamerFunc func(playlistPath string) Streamer

// PlayItem is the next file a per-file playout supervisor should run: the file, the seek
// offset into it, and the media parameters probed from the file for the transcode script.
type PlayItem struct {
	File           string
	Offset         float64
	NextBoundary   time.Time
	VideoCodec     string
	Width          int
	Height         int
	AspectWidth    int
	PixelFormat    string
	FrameRate      string
	FieldOrder     string
	AudioCodec     string
	AudioChannels  int
	SampleRate     int
	AudioLanguages []string
}

// NextItemFunc resolves the file to play at the current time. ok=false means no playable
// item is available yet (the supervisor backs off and retries).
type NextItemFunc func(now time.Time) (PlayItem, bool)

type Request struct {
	StreamKey      string
	StreamURL      string
	ExtraVars      map[string]any
	ClientStreamer ClientStreamerFunc
	Semaphore      *semaphore.Weighted
	Runner         RunnerSpec
	// NextItem, when set, switches the segmenter to per-file playout: it runs one process
	// per resolved file instead of a single long-lived command.
	NextItem NextItemFunc
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
	clientCtx := ctxutil.WithStreamID(ctx, hashid.New(req.StreamKey))
	streamCtx := context.WithoutCancel(clientCtx)

	seg, isNew, err := d.pool.getOrCreate(streamCtx, d.baseDir, req)
	if err != nil {
		return nil, err
	}

	if isNew {
		if !utils.AcquireSemaphore(streamCtx, req.Semaphore, semaphoreTimeout, "subscription") {
			d.pool.remove(seg)
			return nil, ErrSubscriptionSemaphore
		}
		logging.Debug(streamCtx, "acquired subscription semaphore")
		go d.runSegmenter(streamCtx, req, seg)
		logging.Info(streamCtx, "started new segmenter")
	} else {
		metrics.IncStreamsReused(ctx)
		logging.Info(clientCtx, "joined existing segmenter")
	}

	select {
	case <-seg.ready:
	case <-clientCtx.Done():
		d.pool.leave(seg)
		return nil, clientCtx.Err()
	}

	if seg.startErr != nil {
		d.pool.leave(seg)
		return nil, fmt.Errorf("%w: %v", ErrSegmenterFailed, seg.startErr)
	}

	cs := newClientStream(clientCtx, req.ClientStreamer(seg.playlistPath))

	return &clientReader{
		clientStream: cs,
		pool:         d.pool,
		seg:          seg,
	}, nil
}

func (d *StreamPool) runSegmenter(ctx context.Context, req Request, seg *segmenter) {
	metrics.IncPlaylistStreamsActive(ctx)
	defer metrics.DecPlaylistStreamsActive(ctx)
	defer d.pool.remove(seg)
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

	seg.start(segCtx)

	<-seg.waitEmpty()
}

func segmentDir(baseDir, streamKey string) string {
	return filepath.Join(baseDir, hashid.New(streamKey))
}
