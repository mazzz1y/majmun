package streampool

import "sync"

type clientReader struct {
	*clientStream
	pool *segmenterPool
	seg  *segmenter
	once sync.Once
}

func (cr *clientReader) Close() error {
	var err error
	cr.once.Do(func() {
		err = cr.clientStream.Close()
		cr.pool.leave(cr.seg)
	})
	return err
}
