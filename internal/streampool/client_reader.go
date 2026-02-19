package streampool

import "sync"

type clientReader struct {
	*clientStream
	seg  *segmenter
	once sync.Once
}

func (cr *clientReader) Close() error {
	var err error
	cr.once.Do(func() {
		err = cr.clientStream.Close()
		cr.seg.removeClient()
	})
	return err
}
