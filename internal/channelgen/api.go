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
	PixelFormat    string
	FrameRate      string
	FieldOrder     string
	AudioCodec     string
	AudioChannels  int
	SampleRate     int
	AudioLanguages []string
}

// ResolveCurrent returns the file to play at now without blocking on a scan or probe. It
// returns ok=false while the schedule is still building (caller serves a placeholder) or
// when the channel has no playable items. Missing files are skipped (offset resets to the
// start of the next surviving file) and trigger a background rebuild while playback
// continues.
func (c *Channel) ResolveCurrent(now time.Time) (PlayItem, bool) {
	s := c.current(now)
	if s == nil {
		return PlayItem{}, false
	}

	index, _, offset, boundary, ok := locate(s, now)
	if !ok {
		return PlayItem{}, false
	}

	n := len(s.Items)
	for k := range n {
		it := s.Items[(index+k)%n]
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
			PixelFormat:    it.PixelFormat,
			FrameRate:      it.FrameRate,
			FieldOrder:     it.FieldOrder,
			AudioCodec:     it.AudioCodec,
			AudioChannels:  it.AudioChannels,
			SampleRate:     it.SampleRate,
			AudioLanguages: it.AudioLanguages,
		}, true
	}

	return PlayItem{}, false
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

	elapsed := float64(start.Unix() - s.Anchor)
	cycles := int64(math.Floor(elapsed / total))
	cycleStart := s.Anchor + cycles*int64(total)

	var programmes []Programme
	cursor := time.Unix(cycleStart, 0)
	for cursor.Before(end) {
		for _, it := range s.Items {
			slotStart := cursor
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
