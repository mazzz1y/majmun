package streampool

import (
	"context"
	"io"
)

type clientStream struct {
	pr     *io.PipeReader
	cancel context.CancelFunc
	done   chan struct{}
}

func newClientStream(ctx context.Context, streamer Streamer) *clientStream {
	ctx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() { _ = pw.Close() }()
		_, _ = streamer.RunWithStdout(ctx, pw)
	}()

	return &clientStream{
		pr:     pr,
		cancel: cancel,
		done:   done,
	}
}

func (cs *clientStream) Read(p []byte) (int, error) {
	return cs.pr.Read(p)
}

func (cs *clientStream) Close() error {
	cs.cancel()
	err := cs.pr.Close()
	<-cs.done
	return err
}
