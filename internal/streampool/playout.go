package streampool

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	// playoutBackoff is the wait before retrying when no item resolves (channel rebuilding)
	// or after a file process exits abnormally, so a broken file can't spin the loop.
	playoutBackoff = time.Second
	// minRunDuration guards against a command that exits near-instantly (misconfigured or
	// crash-looping): a run shorter than this triggers a backoff even on success, since a
	// realtime-paced file always runs for at least several seconds.
	minRunDuration = time.Second
)

// runPlayout drives per-file playout: it resolves the current file, runs one process for it
// with per-file template vars, and on exit advances to the next file. The shared HLS dir is
// written across invocations, so clients read one continuous stream with a discontinuity at
// each file boundary. It returns when ctx is cancelled.
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
				"PixelFormat":    item.PixelFormat,
				"FrameRate":      item.FrameRate,
				"FieldOrder":     item.FieldOrder,
				"AudioCodec":     item.AudioCodec,
				"AudioChannels":  formatInt(item.AudioChannels),
				"SampleRate":     formatInt(item.SampleRate),
				"AudioLanguages": strings.Join(item.AudioLanguages, " "),
			},
		})

		// Readiness is left to waitForSegments, so a fast first-file failure doesn't mark the
		// channel ready with an empty playlist.
		started := time.Now()
		err := runner.Run(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || time.Since(started) < minRunDuration {
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
