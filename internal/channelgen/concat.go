package channelgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type concatWindow struct {
	files  []Item
	offset float64
	dirty  bool
}

// buildConcatWindow returns the full schedule rotated so the current item plays first, with
// offset seeking into it. ffmpeg loops this list, so every file airs in turn and the channel
// wraps forever. Missing files are skipped (marking the schedule dirty for a rebuild); when
// the current item itself is gone, the offset resets so playback starts cleanly on the next
// surviving file.
func buildConcatWindow(s *Schedule, startIndex int, offset float64) concatWindow {
	w := concatWindow{offset: offset}
	n := len(s.Items)
	if n == 0 {
		w.dirty = true
		return w
	}

	first := true
	for k := range n {
		it := s.Items[(startIndex+k)%n]
		if !fileExists(it.File) {
			w.dirty = true
			if first {
				w.offset = 0
			}
			continue
		}
		w.files = append(w.files, it)
		first = false
	}

	return w
}

// writeConcatList writes an ffmpeg concat list seeking into the first file via inpoint. Unlike
// an input -ss, inpoint opens the file normally so the decoder reads its codec headers (e.g.
// the AV1 sequence header) before seeking, which is required for mid-file joins to decode.
func writeConcatList(dir string, files []Item, offset float64) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "concat.txt")

	var b strings.Builder
	for i, it := range files {
		fmt.Fprintf(&b, "file '%s'\n", escapeConcatPath(it.File))
		if i == 0 && offset > 0 {
			fmt.Fprintf(&b, "inpoint %.3f\n", offset)
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func escapeConcatPath(path string) string {
	return strings.ReplaceAll(path, "'", `'\''`)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
