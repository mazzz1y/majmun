package demux

import (
	"io"
	"majmun/internal/ioutil"
	"sync"
)

const (
	bufferSize = 32 * 1024 * 1024
)

type StreamWriter struct {
	clients     map[io.WriteCloser]*ioutil.AsyncWriter
	clientsLock sync.RWMutex

	buffer     []byte
	bufferPos  int
	bufferFull bool
	bufferLock sync.RWMutex

	emptyNotify     chan struct{}
	notifyListeners sync.Map
}

func NewStreamWriter() *StreamWriter {
	return &StreamWriter{
		clients:     make(map[io.WriteCloser]*ioutil.AsyncWriter),
		buffer:      make([]byte, bufferSize),
		bufferPos:   0,
		bufferFull:  false,
		emptyNotify: make(chan struct{}),
	}
}

func (sw *StreamWriter) AddClient(w io.WriteCloser) {
	sw.clientsLock.Lock()
	defer sw.clientsLock.Unlock()

	cw := ioutil.NewAsyncWriter(w)
	sw.clients[w] = cw

	sw.bufferLock.RLock()
	defer sw.bufferLock.RUnlock()

	if !sw.bufferFull && sw.bufferPos == 0 {
		return
	}

	if sw.bufferFull {
		fullBuffer := make([]byte, bufferSize)
		copy(fullBuffer, sw.buffer[sw.bufferPos:])
		copy(fullBuffer[len(sw.buffer)-sw.bufferPos:], sw.buffer[:sw.bufferPos])
		cw.Write(fullBuffer)
	} else {
		validData := make([]byte, sw.bufferPos)
		copy(validData, sw.buffer[:sw.bufferPos])
		cw.Write(validData)
	}
}

func (sw *StreamWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	data := make([]byte, len(p))
	copy(data, p)

	sw.bufferLock.Lock()
	if len(data) >= bufferSize {
		copy(sw.buffer, data[len(data)-bufferSize:])
		sw.bufferPos = 0
		sw.bufferFull = true
	} else {
		remaining := bufferSize - sw.bufferPos
		if len(data) <= remaining {
			copy(sw.buffer[sw.bufferPos:], data)
			sw.bufferPos += len(data)
			if sw.bufferPos == bufferSize {
				sw.bufferPos = 0
				sw.bufferFull = true
			}
		} else {
			copy(sw.buffer[sw.bufferPos:], data[:remaining])
			copy(sw.buffer, data[remaining:])
			sw.bufferPos = len(data) - remaining
			sw.bufferFull = true
		}
	}
	sw.bufferLock.Unlock()

	sw.clientsLock.RLock()
	clientCount := len(sw.clients)

	if clientCount == 0 {
		sw.clientsLock.RUnlock()
		return len(data), nil
	}

	for _, cw := range sw.clients {
		cw.Write(data)
	}
	sw.clientsLock.RUnlock()

	return len(data), nil
}

func (sw *StreamWriter) RemoveClient(w io.WriteCloser) {
	sw.clientsLock.Lock()
	defer sw.clientsLock.Unlock()

	cw, exists := sw.clients[w]
	if !exists {
		return
	}

	cw.Close()
	delete(sw.clients, w)

	if len(sw.clients) == 0 {
		sw.notifyEmpty()
	}
}

func (sw *StreamWriter) Close() {
	sw.clientsLock.Lock()
	defer sw.clientsLock.Unlock()

	for client, cw := range sw.clients {
		cw.Close()
		_ = client.Close()
		delete(sw.clients, client)
	}

	if len(sw.clients) == 0 {
		sw.notifyEmpty()
	}
}

func (sw *StreamWriter) ClientCount() int {
	sw.clientsLock.RLock()
	defer sw.clientsLock.RUnlock()
	return len(sw.clients)
}

func (sw *StreamWriter) IsEmpty() bool {
	sw.clientsLock.RLock()
	defer sw.clientsLock.RUnlock()
	return len(sw.clients) == 0
}

func (sw *StreamWriter) CancelEmptyChannel(ch <-chan struct{}) {
	sw.notifyListeners.Delete(ch)
}

func (sw *StreamWriter) IsEmptyChannel() <-chan struct{} {
	notifyCh := make(chan struct{})
	sw.notifyListeners.Store(notifyCh, struct{}{})
	return notifyCh
}

func (sw *StreamWriter) notifyEmpty() {
	sw.notifyListeners.Range(func(key, value any) bool {
		ch, ok := key.(chan struct{})
		if ok {
			close(ch)
			sw.notifyListeners.Delete(key)
		}
		return true
	})
}
