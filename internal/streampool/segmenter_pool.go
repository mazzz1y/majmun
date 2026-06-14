package streampool

import (
	"context"
	"fmt"
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

func (p *segmenterPool) getOrCreate(ctx context.Context, baseDir string, req Request) (*segmenter, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if seg, exists := p.segmenters[req.StreamKey]; exists {
		if seg.join() {
			return seg, false, nil
		}
		delete(p.segmenters, req.StreamKey)
	}

	seg, err := newSegmenter(ctx, baseDir, req)
	if err != nil {
		return nil, false, fmt.Errorf("create segmenter: %w", err)
	}

	p.segmenters[req.StreamKey] = seg
	return seg, true, nil
}

func (p *segmenterPool) leave(seg *segmenter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	seg.removeClient()
}

// remove tears seg down (cancel its process, delete its own unique segment dir) and drops it
// from the map only if it is still the segmenter registered under its key. A drained
// segmenter may already have been replaced by a fresh one at the same key; the identity check
// keeps that replacement in the map. Tearing seg down is still safe in that case because each
// segmenter owns a distinct dir, so cleaning seg can never touch the replacement's segments.
func (p *segmenterPool) remove(seg *segmenter) {
	p.mu.Lock()
	if current, exists := p.segmenters[seg.streamKey]; exists && current == seg {
		delete(p.segmenters, seg.streamKey)
	}
	p.mu.Unlock()

	seg.stop()
	seg.cleanup()
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
