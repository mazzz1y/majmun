package streampool

import (
	"context"
	"fmt"
	"io"
	"majmun/internal/config/proxy"
	"majmun/internal/shell"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const segmentReadyPoll = 100 * time.Millisecond

type segmenter struct {
	streamKey   string
	dir         string
	playlistURL string
	config      proxy.Segmenter
	streamer    *shell.Streamer

	ctx    context.Context
	cancel context.CancelFunc

	clientCount atomic.Int64
	emptyChan   chan struct{}
	emptyOnce   sync.Once

	ready     chan struct{}
	readyOnce sync.Once
	startErr  error
}

func newSegmenter(parentCtx context.Context, streamKey string, baseDir string, cfg proxy.Segmenter) (*segmenter, error) {
	dir := filepath.Join(baseDir, streamKey)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create segment dir: %w", err)
	}

	playlistURL := filepath.Join(dir, "stream.m3u8")

	base, err := shell.NewShellStreamer(cfg.Command, cfg.EnvVars, cfg.TemplateVars)
	if err != nil {
		return nil, fmt.Errorf("parse segmenter command: %w", err)
	}

	streamer := base.WithTemplateVars(map[string]any{
		"segment_duration": fmt.Sprintf("%d", *cfg.SegmentDuration),
		"max_segments":     fmt.Sprintf("%d", *cfg.MaxSegments),
		"segment_path":     filepath.Join(dir, "seg_%05d.ts"),
		"playlist_path":    playlistURL,
	})

	ctx, cancel := context.WithCancel(parentCtx)

	s := &segmenter{
		streamKey:   streamKey,
		dir:         dir,
		playlistURL: playlistURL,
		config:      cfg,
		streamer:    streamer,
		ctx:         ctx,
		cancel:      cancel,
		emptyChan:   make(chan struct{}),
		ready:       make(chan struct{}),
	}

	s.clientCount.Store(1)

	return s, nil
}

func (s *segmenter) start(upstream io.Reader) {
	go s.waitForSegments()

	err := s.streamer.RunWithStdin(s.ctx, upstream)
	if err != nil && s.ctx.Err() == nil {
		s.startErr = fmt.Errorf("segmenter process: %w", err)
	}
	s.closeReady()
}

func (s *segmenter) closeReady() {
	s.readyOnce.Do(func() {
		close(s.ready)
	})
}

func (s *segmenter) waitForSegments() {
	deadline := time.After(time.Duration(*s.config.ReadyTimeout))
	ticker := time.NewTicker(segmentReadyPoll)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.startErr = s.ctx.Err()
			s.closeReady()
			return
		case <-deadline:
			s.startErr = fmt.Errorf("timeout waiting for segments")
			s.closeReady()
			return
		case <-ticker.C:
			if s.countSegments() >= *s.config.InitSegments {
				s.closeReady()
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

func (s *segmenter) addClient() {
	s.clientCount.Add(1)
}

func (s *segmenter) removeClient() {
	if s.clientCount.Add(-1) <= 0 {
		s.notifyEmpty()
	}
}

func (s *segmenter) waitEmpty() <-chan struct{} {
	return s.emptyChan
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
		close(s.emptyChan)
	})
}
