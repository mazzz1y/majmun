package streampool

import (
	"bufio"
	"context"
	"io"
	"majmun/internal/logging"
	"os/exec"
	"sync"
)

type clientStream struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func newClientStream(ctx context.Context, playlistPath string) (*clientStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-v", "fatal",
		"-live_start_index", "-1",
		"-i", playlistPath,
		"-c", "copy",
		"-f", "mpegts",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logging.Debug(ctx, "client ffmpeg", "msg", scanner.Text())
		}
	}()

	return &clientStream{
		cmd:    cmd,
		stdout: stdout,
		cancel: cancel,
	}, nil
}

func (cs *clientStream) Read(p []byte) (int, error) {
	return cs.stdout.Read(p)
}

func (cs *clientStream) Close() error {
	var err error
	cs.once.Do(func() {
		cs.cancel()
		err = cs.cmd.Wait()
	})
	return err
}
