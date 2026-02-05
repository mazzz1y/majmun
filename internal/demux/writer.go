package demux

import (
	"io"
	"majmun/internal/ioutil"
	"sync"
)

const bufferSize = 32 * 1024 * 1024

type StreamWriter struct {
	clients     map[io.WriteCloser]*ioutil.AsyncWriter
	clientsLock sync.RWMutex

	buffer     []byte
	bufferPos  int
	bufferFull bool
	bufferLock sync.Mutex

	emptyChan chan struct{}
	emptyOnce sync.Once
}

func NewStreamWriter() *StreamWriter {
	return &StreamWriter{
		clients:   make(map[io.WriteCloser]*ioutil.AsyncWriter),
		buffer:    make([]byte, bufferSize),
		emptyChan: make(chan struct{}),
	}
}

func (sw *StreamWriter) AddClient(w io.WriteCloser) {
	sw.clientsLock.Lock()
	cw := ioutil.NewAsyncWriter(w)
	sw.clients[w] = cw
	sw.clientsLock.Unlock()

	sw.bufferLock.Lock()
	pos := sw.bufferPos
	full := sw.bufferFull
	var data []byte
	if full {
		data = make([]byte, bufferSize)
		copy(data, sw.buffer[pos:])
		copy(data[bufferSize-pos:], sw.buffer[:pos])
	} else if pos > 0 {
		data = make([]byte, pos)
		copy(data, sw.buffer[:pos])
	}
	sw.bufferLock.Unlock()

	if len(data) > 0 {
		cw.Write(data)
	}
}

func (sw *StreamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	sw.bufferLock.Lock()
	if len(p) >= bufferSize {
		copy(sw.buffer, p[len(p)-bufferSize:])
		sw.bufferPos = 0
		sw.bufferFull = true
	} else {
		remaining := bufferSize - sw.bufferPos
		if len(p) <= remaining {
			copy(sw.buffer[sw.bufferPos:], p)
			sw.bufferPos += len(p)
			if sw.bufferPos == bufferSize {
				sw.bufferPos = 0
				sw.bufferFull = true
			}
		} else {
			copy(sw.buffer[sw.bufferPos:], p[:remaining])
			copy(sw.buffer, p[remaining:])
			sw.bufferPos = len(p) - remaining
			sw.bufferFull = true
		}
	}
	sw.bufferLock.Unlock()

	sw.clientsLock.RLock()
	for _, cw := range sw.clients {
		cw.Write(p)
	}
	sw.clientsLock.RUnlock()

	return len(p), nil
}

func (sw *StreamWriter) RemoveClient(w io.WriteCloser) {
	sw.clientsLock.Lock()
	cw, exists := sw.clients[w]
	if !exists {
		sw.clientsLock.Unlock()
		return
	}
	delete(sw.clients, w)
	empty := len(sw.clients) == 0
	sw.clientsLock.Unlock()

	cw.Close()

	if empty {
		sw.notifyEmpty()
	}
}

func (sw *StreamWriter) Close() {
	sw.clientsLock.Lock()
	clients := sw.clients
	sw.clients = make(map[io.WriteCloser]*ioutil.AsyncWriter)
	sw.clientsLock.Unlock()

	for client, cw := range clients {
		cw.Close()
		_ = client.Close()
	}

	sw.notifyEmpty()
}

func (sw *StreamWriter) WaitEmpty() <-chan struct{} {
	return sw.emptyChan
}

func (sw *StreamWriter) notifyEmpty() {
	sw.emptyOnce.Do(func() {
		close(sw.emptyChan)
	})
}
