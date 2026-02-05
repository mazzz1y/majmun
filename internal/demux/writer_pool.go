package demux

import (
	"io"
	"sync"
)

type WriterPool struct {
	writers map[string]*StreamWriter
	mutex   sync.Mutex
}

func NewWriterPool() *WriterPool {
	return &WriterPool{
		writers: make(map[string]*StreamWriter),
	}
}

func (p *WriterPool) Stop() {
	p.mutex.Lock()
	writers := p.writers
	p.writers = make(map[string]*StreamWriter)
	p.mutex.Unlock()

	for _, writer := range writers {
		writer.Close()
	}
}

func (p *WriterPool) AddClient(streamKey string, client io.WriteCloser) (*StreamWriter, bool) {
	p.mutex.Lock()
	writer, exists := p.writers[streamKey]
	if !exists {
		writer = NewStreamWriter()
		p.writers[streamKey] = writer
	}
	p.mutex.Unlock()

	writer.AddClient(client)
	return writer, !exists
}

func (p *WriterPool) RemoveClient(streamKey string, client io.WriteCloser) {
	p.mutex.Lock()
	writer, exists := p.writers[streamKey]
	p.mutex.Unlock()

	if exists {
		writer.RemoveClient(client)
	}
}

func (p *WriterPool) CloseStream(streamKey string) {
	p.mutex.Lock()
	writer := p.writers[streamKey]
	delete(p.writers, streamKey)
	p.mutex.Unlock()

	if writer != nil {
		writer.Close()
	}
}
