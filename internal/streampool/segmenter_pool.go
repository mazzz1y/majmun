package streampool

import (
	"context"
	"fmt"
	"majmun/internal/config/proxy"
	"sync"
)

type segmenterPool struct {
	segmenters map[string]*segmenter
	mu         sync.Mutex
}

func newSegmenterPool() *segmenterPool {
	return &segmenterPool{
		segmenters: make(map[string]*segmenter),
	}
}

func (p *segmenterPool) getOrCreate(streamKey string, ctx context.Context, baseDir string, cfg proxy.Segmenter) (*segmenter, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if seg, exists := p.segmenters[streamKey]; exists {
		return seg, false, nil
	}

	seg, err := newSegmenter(ctx, streamKey, baseDir, cfg)
	if err != nil {
		return nil, false, fmt.Errorf("create segmenter: %w", err)
	}

	p.segmenters[streamKey] = seg
	return seg, true, nil
}

func (p *segmenterPool) remove(streamKey string) {
	p.mu.Lock()
	seg, exists := p.segmenters[streamKey]
	if exists {
		delete(p.segmenters, streamKey)
	}
	p.mu.Unlock()

	if exists && seg != nil {
		seg.stop()
		seg.cleanup()
	}
}

func (p *segmenterPool) stopAll() {
	p.mu.Lock()
	segmenters := p.segmenters
	p.segmenters = make(map[string]*segmenter)
	p.mu.Unlock()

	for _, seg := range segmenters {
		seg.stop()
		seg.cleanup()
	}
}
