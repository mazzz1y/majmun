package streampool

import (
	"context"
	"fmt"
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
	streamKey    string
	dir          string
	playlistPath string
	config       proxy.Segmenter
	streamer     *shell.Streamer

	ctx    context.Context
	cancel context.CancelFunc

	clientCount atomic.Int64
	emptyChan   chan struct{}
	emptyOnce   sync.Once

	ready     chan struct{}
	readyOnce sync.Once
	startErr  error
}

func newSegmenter(parentCtx context.Context, streamKey string, baseDir string, cfg proxy.Segmenter, streamURL string) (*segmenter, error) {
	dir := filepath.Join(baseDir, streamKey)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create segment dir: %w", err)
	}

	playlistPath := filepath.Join(dir, "stream.m3u8")

	base, err := shell.NewShellStreamer(cfg.Command, cfg.EnvVars, cfg.TemplateVars)
	if err != nil {
		return nil, fmt.Errorf("parse segmenter command: %w", err)
	}

	streamer := base.WithTemplateVars(map[string]any{
		"url":           streamURL,
		"segment_path":  filepath.Join(dir, "seg_%05d.ts"),
		"playlist_path": playlistPath,
	})

	ctx, cancel := context.WithCancel(parentCtx)

	s := &segmenter{
		streamKey:    streamKey,
		dir:          dir,
		playlistPath: playlistPath,
		config:       cfg,
		streamer:     streamer,
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

	err := s.streamer.Run(ctx)
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
	deadline := time.After(time.Duration(*s.config.ReadyTimeout))
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
			if s.countSegments() >= *s.config.InitSegments {
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
