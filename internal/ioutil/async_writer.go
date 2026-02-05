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
	closeOnce sync.Once
}

func NewAsyncWriter(client io.Writer) *AsyncWriter {
	cw := &AsyncWriter{
		client:   client,
		dataChan: make(chan []byte, asyncBufferSize),
		doneChan: make(chan struct{}),
	}
	go cw.writeLoop()
	return cw
}

func (cw *AsyncWriter) writeLoop() {
	defer close(cw.doneChan)
	for data := range cw.dataChan {
		if _, err := cw.client.Write(data); err != nil {
			break
		}
	}
}

func (cw *AsyncWriter) Write(data []byte) {
	select {
	case cw.dataChan <- data:
	default:
		select {
		case <-cw.dataChan:
		default:
		}
		select {
		case cw.dataChan <- data:
		default:
		}
	}
}

func (cw *AsyncWriter) Close() {
	cw.closeOnce.Do(func() {
		close(cw.dataChan)
	})
	<-cw.doneChan
}
