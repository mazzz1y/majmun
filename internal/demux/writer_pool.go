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
	defer p.mutex.Unlock()

	for key, writer := range p.writers {
		writer.Close()
		delete(p.writers, key)
	}
}

func (p *WriterPool) AddClient(streamKey string, client io.WriteCloser) (*StreamWriter, bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	writer, exists := p.writers[streamKey]
	if exists {
		writer.AddClient(client)
		return writer, false
	}

	writer = NewStreamWriter()
	p.writers[streamKey] = writer
	writer.AddClient(client)
	return writer, true
}

func (p *WriterPool) RemoveClient(streamKey string, client io.WriteCloser) {
	p.mutex.Lock()
	writer, exists := p.writers[streamKey]
	p.mutex.Unlock()

	if exists {
		writer.RemoveClient(client)
	}
}

func (p *WriterPool) RemoveAndClose(streamKey string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	writer := p.writers[streamKey]
	delete(p.writers, streamKey)
	if writer != nil {
		writer.Close()
	}
}
