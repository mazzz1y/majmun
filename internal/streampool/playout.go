package streampool

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	// playoutBackoff is the wait before retrying after no item resolves or a process exits abnormally.
	playoutBackoff = time.Second
	// minRunDuration backs off after a suspiciously short successful run, guarding a crash-loop.
	minRunDuration = time.Second
)

// runPlayout runs one process per resolved file into the shared HLS dir, advancing on each exit so
// clients read one continuous stream with a discontinuity at each boundary. Returns on ctx cancel.
func (s *segmenter) runPlayout(ctx context.Context) error {
	for ctx.Err() == nil {
		item, ok := s.nextItem(time.Now())
		if !ok {
			if !sleep(ctx, playoutBackoff) {
				return ctx.Err()
			}
			continue
		}

		runner := s.streamer.WithTemplateVars(map[string]any{
			"Playout": map[string]any{
				"Input":          item.File,
				"Offset":         formatSeconds(item.Offset),
				"VideoCodec":     item.VideoCodec,
				"Width":          formatInt(item.Width),
				"Height":         formatInt(item.Height),
				"AspectWidth":    formatInt(item.AspectWidth),
				"PixelFormat":    item.PixelFormat,
				"FrameRate":      item.FrameRate,
				"FieldOrder":     item.FieldOrder,
				"AudioCodec":     item.AudioCodec,
				"AudioChannels":  formatInt(item.AudioChannels),
				"SampleRate":     formatInt(item.SampleRate),
				"AudioLanguages": strings.Join(item.AudioLanguages, " "),
			},
		})

		// Readiness is left to waitForSegments, so a fast first-file failure can't mark ready empty.
		started := time.Now()
		err := runner.Run(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			if !sleep(ctx, playoutBackoff) {
				return ctx.Err()
			}
			continue
		}

		if !item.NextBoundary.IsZero() && time.Now().Before(item.NextBoundary) {
			if !sleepUntil(ctx, item.NextBoundary) {
				return ctx.Err()
			}
			continue
		}

		if time.Since(started) < minRunDuration {
			if !sleep(ctx, playoutBackoff) {
				return ctx.Err()
			}
		}
	}
	return ctx.Err()
}

func formatSeconds(sec float64) string {
	if sec <= 0 {
		return ""
	}
	return strconv.FormatFloat(sec, 'f', 3, 64)
}

// formatInt renders a probed numeric field, returning "" for the zero value so an unknown
// parameter is omitted from the environment rather than exported as "0".
func formatInt(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// sleepUntil waits until t, returning false if ctx was cancelled first.
func sleepUntil(ctx context.Context, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return true
	}
	return sleep(ctx, d)
}

// sleep returns false if ctx was cancelled before d elapsed.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
