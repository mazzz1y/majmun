package listing

import (
	"context"
	"io"
	"sync"
	"time"
)

const (
	bufferTicker    = time.Second
	bufferBatchSize = 1000
	bufferSize      = 100000
)

type initFunc func(ctx context.Context, url string) (Decoder, io.ReadCloser, error)

type BaseDecoder struct {
	decoder      Decoder
	reader       io.ReadCloser
	err          error
	itemBuffer   []any
	bufferCtx    context.Context
	cancelBuffer context.CancelFunc
	url          string
	initFunc     initFunc
	bufferWG     sync.WaitGroup
}

func NewLazyBaseDecoder(url string, init initFunc) *BaseDecoder {
	return &BaseDecoder{
		url:        url,
		initFunc:   init,
		itemBuffer: make([]any, 0, bufferSize),
	}
}

func (d *BaseDecoder) NextItem() (any, error) {
	if len(d.itemBuffer) > 0 {
		item := d.itemBuffer[0]
		d.itemBuffer = d.itemBuffer[1:]
		return item, nil
	}

	if d.err != nil {
		return nil, d.err
	}

	return d.decoder.Decode()
}

func (d *BaseDecoder) StartBuffering(ctx context.Context) error {
	if err := d.init(ctx); err != nil {
		d.err = err
		return err
	}

	d.bufferCtx, d.cancelBuffer = context.WithCancel(ctx)

	d.bufferWG.Go(func() {
		batch := make([]any, 0, bufferBatchSize)
		ticker := time.NewTicker(bufferTicker)

		defer func() {
			d.cancelBuffer()
			ticker.Stop()
			if len(batch) > 0 {
				d.AddToBuffer(batch...)
				batch = batch[:0]
			}
		}()

		for {
			select {
			case <-d.bufferCtx.Done():
				return

			case <-ticker.C:
				for len(batch) < bufferBatchSize {
					item, err := d.decoder.Decode()
					if err != nil {
						d.err = err
						return
					}
					batch = append(batch, item)
				}
			}
		}
	})

	return nil
}

func (d *BaseDecoder) AddToBuffer(items ...any) {
	d.itemBuffer = append(d.itemBuffer, items...)
}

func (d *BaseDecoder) StopBuffer() {
	if d.cancelBuffer != nil {
		d.cancelBuffer()
	}
	d.bufferWG.Wait()
}

func (d *BaseDecoder) Close() error {
	if d.cancelBuffer != nil {
		d.cancelBuffer()
	}
	if d.reader != nil {
		return d.reader.Close()
	}
	return nil
}

func (d *BaseDecoder) init(ctx context.Context) error {
	if d.decoder != nil || d.reader != nil {
		return nil
	}

	decoder, reader, err := d.initFunc(ctx, d.url)
	if err != nil {
		return err
	}

	d.decoder = decoder
	d.reader = reader

	return nil
}
