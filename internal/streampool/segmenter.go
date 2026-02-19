package streampool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"majmun/internal/logging"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	hlsSegmentDuration = 2
	hlsListSize        = 15
	segmentReadyPoll   = 100 * time.Millisecond
	segmentReadyMax    = 30 * time.Second
)

type segmenter struct {
	streamKey   string
	dir         string
	playlistURL string

	ctx    context.Context
	cancel context.CancelFunc

	clientCount atomic.Int64
	emptyChan   chan struct{}
	emptyOnce   sync.Once

	ready    chan struct{}
	startErr error
}

func newSegmenter(parentCtx context.Context, streamKey string, baseDir string) (*segmenter, error) {
	dir := filepath.Join(baseDir, streamKey)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create segment dir: %w", err)
	}

	ctx, cancel := context.WithCancel(parentCtx)

	s := &segmenter{
		streamKey:   streamKey,
		dir:         dir,
		playlistURL: filepath.Join(dir, "stream.m3u8"),
		ctx:         ctx,
		cancel:      cancel,
		emptyChan:   make(chan struct{}),
		ready:       make(chan struct{}),
	}

	s.clientCount.Store(1)

	return s, nil
}

func (s *segmenter) start(upstream io.Reader) {
	cmd := exec.CommandContext(s.ctx,
		"ffmpeg",
		"-v", "fatal",
		"-i", "pipe:0",
		"-c", "copy",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", hlsSegmentDuration),
		"-hls_list_size", fmt.Sprintf("%d", hlsListSize),
		"-hls_flags", "delete_segments+append_list+independent_segments",
		"-hls_segment_filename", filepath.Join(s.dir, "seg_%05d.ts"),
		s.playlistURL,
	)

	cmd.Stdin = upstream

	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.startErr = fmt.Errorf("stderr pipe: %w", err)
		close(s.ready)
		return
	}

	if err := cmd.Start(); err != nil {
		s.startErr = fmt.Errorf("start segmenter: %w", err)
		close(s.ready)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logging.Debug(s.ctx, "segmenter ffmpeg", "stream", s.streamKey, "msg", scanner.Text())
		}
	}()

	s.waitForPlaylist()

	_ = cmd.Wait()
}

func (s *segmenter) waitForPlaylist() {
	deadline := time.After(segmentReadyMax)
	ticker := time.NewTicker(segmentReadyPoll)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.startErr = s.ctx.Err()
			close(s.ready)
			return
		case <-deadline:
			s.startErr = fmt.Errorf("timeout waiting for first segment")
			close(s.ready)
			return
		case <-ticker.C:
			if _, err := os.Stat(s.playlistURL); err == nil {
				close(s.ready)
				return
			}
		}
	}
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
