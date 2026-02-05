package ioutil

import (
	"io"
	"sync"
)

const asyncBufferSize = 64

type AsyncWriter struct {
	client    io.Writer
	dataChan  chan []byte
	doneChan  chan struct{}
	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
}

func NewAsyncWriter(client io.Writer) *AsyncWriter {
	aw := &AsyncWriter{
		client:   client,
		dataChan: make(chan []byte, asyncBufferSize),
		doneChan: make(chan struct{}),
	}
	go aw.writeLoop()
	return aw
}

func (aw *AsyncWriter) writeLoop() {
	defer close(aw.doneChan)
	for data := range aw.dataChan {
		if _, err := aw.client.Write(data); err != nil {
			aw.mu.Lock()
			aw.closed = true
			aw.mu.Unlock()
			return
		}
	}
}

func (aw *AsyncWriter) Write(data []byte) {
	aw.mu.RLock()
	defer aw.mu.RUnlock()

	if aw.closed {
		return
	}

	buf := make([]byte, len(data))
	copy(buf, data)

	select {
	case aw.dataChan <- buf:
	default:
		select {
		case <-aw.dataChan:
		default:
		}
		select {
		case aw.dataChan <- buf:
		default:
		}
	}
}

func (aw *AsyncWriter) Close() {
	aw.closeOnce.Do(func() {
		aw.mu.Lock()
		aw.closed = true
		close(aw.dataChan)
		aw.mu.Unlock()
	})
	<-aw.doneChan
}
