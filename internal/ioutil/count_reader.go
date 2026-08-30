package ioutil

import (
	"io"
	"sync/atomic"
)

type CountReadCloser struct {
	src     io.ReadCloser
	counter *atomic.Int64
}

func NewCountReadCloser(src io.ReadCloser, counter *atomic.Int64) *CountReadCloser {
	return &CountReadCloser{
		src:     src,
		counter: counter,
	}
}

func (sc *CountReadCloser) Read(p []byte) (n int, err error) {
	if sc.src == nil {
		return 0, io.EOF
	}

	n, err = sc.src.Read(p)

	if sc.counter != nil {
		sc.counter.Add(int64(n))
	}
	return
}

func (sc *CountReadCloser) Close() error {
	if sc.src != nil {
		return sc.src.Close()
	}
	return nil
}
