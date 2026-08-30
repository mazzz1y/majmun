package streampool

import (
	"context"
	"fmt"
	"majmun/internal/shell"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const segmentReadyPoll = 100 * time.Millisecond

// segmenterSeq makes each segmenter's on-disk dir unique, so a drained segmenter that is
// still shutting down never shares its segment dir with the fresh one that replaced it at
// the same stream key.
var segmenterSeq atomic.Uint64

type segmenter struct {
	streamKey    string
	dir          string
	playlistPath string
	runner       RunnerSpec
	streamer     *shell.Streamer
	nextItem     NextItemFunc

	ctx    context.Context
	cancel context.CancelFunc

	clientCount atomic.Int64
	done        bool // guarded by segmenterPool.mu; mutate only via join/removeClient
	emptyChan   chan struct{}
	emptyOnce   sync.Once
	// emptyAt is written inside emptyOnce before emptyChan closes, so any reader that observed
	// the close sees it.
	emptyAt time.Time

	ready     chan struct{}
	readyOnce sync.Once
	startErr  error
}

func newSegmenter(parentCtx context.Context, baseDir string, req Request) (*segmenter, error) {
	dir := segmentDir(baseDir, req.StreamKey) + fmt.Sprintf("-%d", segmenterSeq.Add(1))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create segment dir: %w", err)
	}

	playlistPath := filepath.Join(dir, "stream.m3u8")

	r := req.Runner
	base, err := shell.NewShellStreamer(r.Command, r.EnvVars, r.TemplateVars)
	if err != nil {
		return nil, fmt.Errorf("parse segmenter command: %w", err)
	}

	stream := map[string]any{
		"URL":          req.StreamURL,
		"SegmentPath":  filepath.Join(dir, "seg_%05d.ts"),
		"PlaylistPath": playlistPath,
	}
	vars := map[string]any{"Stream": stream}
	maps.Copy(vars, req.ExtraVars)

	streamer := base.WithTemplateVars(vars)

	ctx, cancel := context.WithCancel(parentCtx)

	s := &segmenter{
		streamKey:    req.StreamKey,
		dir:          dir,
		playlistPath: playlistPath,
		runner:       r,
		streamer:     streamer,
		nextItem:     req.NextItem,
		ctx:          ctx,
		cancel:       cancel,
		emptyChan:    make(chan struct{}),
		ready:        make(chan struct{}),
	}

	s.clientCount.Store(1)

	return s, nil
}

func (s *segmenter) start(ctx context.Context) {
	go s.waitForSegments()

	var err error
	if s.nextItem != nil {
		err = s.runPlayout(ctx)
	} else {
		err = s.streamer.Run(ctx)
	}
	if err != nil && ctx.Err() == nil {
		s.setReady(fmt.Errorf("segmenter process: %w", err))
	} else {
		s.setReady(nil)
	}
}

func (s *segmenter) setReady(err error) {
	s.readyOnce.Do(func() {
		s.startErr = err
		close(s.ready)
	})
}

func (s *segmenter) waitForSegments() {
	deadline := time.After(s.runner.ReadyTimeout)
	ticker := time.NewTicker(segmentReadyPoll)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.setReady(s.ctx.Err())
			return
		case <-deadline:
			s.setReady(fmt.Errorf("timeout waiting for segments"))
			return
		case <-ticker.C:
			if s.countSegments() >= s.runner.InitSegments {
				s.setReady(nil)
				return
			}
		}
	}
}

func (s *segmenter) countSegments() int {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".ts" {
			count++
		}
	}
	return count
}

func (s *segmenter) join() bool {
	if s.done {
		return false
	}
	s.clientCount.Add(1)
	return true
}

func (s *segmenter) removeClient() {
	if s.clientCount.Add(-1) <= 0 {
		s.done = true
		s.notifyEmpty()
	}
}

func (s *segmenter) waitEmpty() <-chan struct{} {
	return s.emptyChan
}

// stoppedAt is when the segmenter drained. Valid only after waitEmpty has fired.
func (s *segmenter) stoppedAt() time.Time {
	return s.emptyAt
}

func (s *segmenter) stop() {
	s.cancel()
	s.notifyEmpty()
}

func (s *segmenter) cleanup() {
	_ = os.RemoveAll(s.dir)
}

func (s *segmenter) notifyEmpty() {
	s.emptyOnce.Do(func() {
		s.emptyAt = time.Now()
		close(s.emptyChan)
	})
}
