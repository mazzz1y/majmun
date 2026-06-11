package channelgen

import (
	"context"
	"math"
	"time"
)

type Resolution struct {
	Fingerprint string
	files       []Item
	offset      float64
}

// Resolve prepares a playable window for now without blocking on a scan or probe. It
// returns ok=false while the schedule is still building (caller serves a placeholder) or
// when the channel has no playable items. A deleted file in the window triggers a
// background rebuild while playback continues on the surviving files.
func (c *Channel) Resolve(now time.Time) (Resolution, bool) {
	s := c.current(now)
	if s == nil {
		return Resolution{}, false
	}

	index, _, offset, _, ok := locate(s, now)
	if !ok {
		return Resolution{}, false
	}

	window := buildConcatWindow(s, index, offset)
	if window.dirty {
		c.markDirty(now)
	}
	if len(window.files) == 0 {
		return Resolution{}, false
	}

	return Resolution{Fingerprint: s.Fingerprint, files: window.files, offset: window.offset}, true
}

// WriteConcatList writes the resolved window's concat list into dir, returning its path.
func (r Resolution) WriteConcatList(dir string) (string, error) {
	return writeConcatList(dir, r.files, r.offset)
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

// epgPast is the small backfill window so an in-progress programme is shown; the forward
// horizon is the per-channel configured epgDuration.
const epgPast = -time.Hour

func (c *Channel) Programmes(_ context.Context, now time.Time) ([]Programme, error) {
	s := c.current(now)
	if s == nil || s.isEmpty() {
		return nil, nil
	}

	total := s.total()
	start := now.Add(epgPast)
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
			programmes = append(programmes, Programme{
				Title:       it.Title,
				Description: it.Description,
				Category:    it.Category,
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
