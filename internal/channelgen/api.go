package channelgen

import (
	"context"
	"math"
	"os"
	"time"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// PlayItem is the single file the supervisor should play next, with the seek offset into it
// and the media parameters probed from the file so a transcode script can adapt per file.
type PlayItem struct {
	File           string
	Offset         float64
	NextBoundary   time.Time
	VideoCodec     string
	Width          int
	Height         int
	AspectWidth    int
	PixelFormat    string
	FrameRate      string
	FieldOrder     string
	AudioCodec     string
	AudioChannels  int
	SampleRate     int
	AudioLanguages []string
}

// ResolveCurrent returns the file to play at now-shift without blocking on a scan or probe.
// ok=false means the schedule is still building or has no playable items; missing files are
// skipped. As in Playout.Next, only the schedule lookup uses the trailing position; the
// rebuild decisions shared with live viewers stay on real time.
func (c *Channel) ResolveCurrent(now time.Time, shift time.Duration) (PlayItem, bool) {
	s := c.current(now)
	if s == nil {
		return PlayItem{}, false
	}

	index, _, offset, boundary, ok := locate(s, now.Add(-shift))
	if !ok {
		return PlayItem{}, false
	}
	item, _, ok := c.resolveFrom(s, index, offset, boundary, now)
	return item, ok
}

// resolveFrom builds the PlayItem at index, skipping missing files (offset resets to the start of
// the next surviving file, marking the channel dirty). It returns the index it resolved at.
func (c *Channel) resolveFrom(s *Schedule, index int, offset float64, boundary, now time.Time) (PlayItem, int, bool) {
	n := len(s.Items)
	for k := range n {
		i := (index + k) % n
		it := s.Items[i]
		if k > 0 {
			offset = 0
			boundary = boundary.Add(time.Duration(it.Duration * float64(time.Second)))
		}
		if !fileExists(it.File) {
			c.markDirty(now)
			continue
		}
		return PlayItem{
			File:           it.File,
			Offset:         offset,
			NextBoundary:   boundary,
			VideoCodec:     it.VideoCodec,
			Width:          it.Width,
			Height:         it.Height,
			AspectWidth:    it.AspectWidth,
			PixelFormat:    it.PixelFormat,
			FrameRate:      it.FrameRate,
			FieldOrder:     it.FieldOrder,
			AudioCodec:     it.AudioCodec,
			AudioChannels:  it.AudioChannels,
			SampleRate:     it.SampleRate,
			AudioLanguages: it.AudioLanguages,
		}, i, true
	}
	return PlayItem{}, 0, false
}

// Playout locates the first item by wall clock (the live entry point), then advances the schedule
// sequentially, re-clocking only to enter a rebuilt schedule where the old index no longer maps.
//
// It owns the catch-up offset: schedule positions resolve at now-shift, while everything
// crossing the package boundary stays in real time.
type Playout struct {
	ch      *Channel
	shift   time.Duration
	sched   *Schedule
	index   int
	started bool
}

// NewPlayout starts a playout trailing wall clock by shift; zero shift is live.
func (c *Channel) NewPlayout(shift time.Duration) *Playout {
	return &Playout{ch: c, shift: shift}
}

func (p *Playout) Shift() time.Duration {
	return p.shift
}

// Next returns the file to play next, advancing the cursor. now and the returned NextBoundary
// are both wall-clock. ok=false means no playable item yet.
func (p *Playout) Next(now time.Time) (PlayItem, bool) {
	// Rebuild decisions are shared with live viewers, so they run on real time.
	s := p.ch.current(now)
	if s == nil || s.isEmpty() {
		p.started = false
		return PlayItem{}, false
	}

	schedNow := now.Add(-p.shift)

	if p.started && s == p.sched {
		next := (p.index + 1) % len(s.Items)
		boundary := schedNow.Add(time.Duration(s.Items[next].Duration * float64(time.Second)))
		if item, idx, ok := p.ch.resolveFrom(s, next, 0, boundary, now); ok {
			p.index = idx
			return p.toRealTime(item), true
		}
	}

	index, _, offset, boundary, ok := locate(s, schedNow)
	if !ok {
		p.started = false
		return PlayItem{}, false
	}
	item, idx, ok := p.ch.resolveFrom(s, index, offset, boundary, now)
	if !ok {
		p.started = false
		return PlayItem{}, false
	}
	p.sched, p.index, p.started = s, idx, true
	return p.toRealTime(item), true
}

// toRealTime converts a boundary resolved in schedule time back to wall clock.
func (p *Playout) toRealTime(item PlayItem) PlayItem {
	if !item.NextBoundary.IsZero() {
		item.NextBoundary = item.NextBoundary.Add(p.shift)
	}
	return item
}

// ClampStart caps a catch-up start at now (future = live). Past times wrap on the loop.
func (c *Channel) ClampStart(requested, now time.Time) time.Time {
	if requested.After(now) {
		return now
	}
	return requested
}

// CatchupWindow is min(epgDuration, one loop); 0 when no schedule is built.
func (c *Channel) CatchupWindow(now time.Time) time.Duration {
	s := c.current(now)
	if s == nil || s.isEmpty() {
		return 0
	}
	return min(c.epgDuration, time.Duration(s.total()*float64(time.Second)))
}

type Programme struct {
	Title       string
	Description string
	Category    string
	Date        string
	Season      int
	Episode     int
	Start       time.Time
	Stop        time.Time
}

func (c *Channel) Programmes(ctx context.Context, now time.Time) ([]Programme, error) {
	ctx = c.logCtx(ctx)
	s := c.current(now)
	if s == nil || s.isEmpty() {
		return nil, nil
	}

	total := s.total()
	// Backfill matches the advertised catch-up window so EPG-driven clients can rewind it.
	loop := time.Duration(total * float64(time.Second))
	backfill := min(c.epgDuration, loop)
	start := now.Add(-backfill)
	end := now.Add(c.epgDuration)

	// Full float64 precision, matching locate's mathMod: truncating total to int64 drifts the
	// cycle origin off the playout timeline by its fractional second on every elapsed loop.
	elapsed := float64(start.UnixNano())/float64(time.Second) - float64(s.Anchor)
	cycles := math.Floor(elapsed / total)
	cycleStart := float64(s.Anchor) + cycles*total

	var programmes []Programme
	cursor := time.Unix(0, int64(cycleStart*float64(time.Second)))
	for cursor.Before(end) {
		for i := 0; i < len(s.Items); i++ {
			it := s.Items[i]
			slotStart := cursor

			// Collapse a run of consecutive filler items into a single break programme.
			if it.IsFiller {
				j := i
				for j < len(s.Items) && s.Items[j].IsFiller {
					cursor = cursor.Add(time.Duration(s.Items[j].Duration * float64(time.Second)))
					j++
				}
				i = j - 1
				if !cursor.Before(start) {
					vars := c.metadataVars(it)
					programmes = append(programmes, Programme{
						Title:    renderField(ctx, c.filler.TitleTemplate, vars, it.Title, "filler title"),
						Category: renderField(ctx, c.filler.CategoryTemplate, vars, it.Category, "filler category"),
						Start:    slotStart,
						Stop:     cursor,
					})
				}
				if !cursor.Before(end) {
					break
				}
				continue
			}

			slotStop := cursor.Add(time.Duration(it.Duration * float64(time.Second)))
			cursor = slotStop
			if slotStop.Before(start) {
				continue
			}
			vars := c.metadataVars(it)
			programmes = append(programmes, Programme{
				Title:       renderField(ctx, c.titleTmpl, vars, it.Title, "title"),
				Description: renderField(ctx, c.descriptionTmpl, vars, it.Description, "description"),
				Category:    renderField(ctx, c.categoryTmpl, vars, it.Category, "category"),
				Date:        it.Date,
				Season:      it.Season,
				Episode:     it.Episode,
				Start:       slotStart,
				Stop:        slotStop,
			})
			if !cursor.Before(end) {
				break
			}
		}
	}

	return programmes, nil
}
