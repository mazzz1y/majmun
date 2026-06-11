package channelgen

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Season/episode patterns commonly found in tags and file names. Word boundaries keep
// resolutions (1280x720) and codec tokens (x264) from matching.
var (
	seasonEpisodeRegexps = []*regexp.Regexp{
		// S01E05, s01.e05
		regexp.MustCompile(`(?i)s(\d{1,4})[ ._-]?e(\d{1,4})`),
		// 1x05, 01x05
		regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,4})\b`),
		// Season 1 Episode 5
		regexp.MustCompile(`(?i)\bseason[ ._-]?(\d{1,4})\b.*?\bep(?:isode)?[ ._-]?(\d{1,4})\b`),
	}
	episodeOnlyRegexps = []*regexp.Regexp{
		// ep05, ep.05, Episode 5
		regexp.MustCompile(`(?i)\bep(?:isode)?[ ._-]?(\d{1,4})\b`),
		// E05, e5
		regexp.MustCompile(`(?i)\be[ ._-]?(\d{1,4})\b`),
	}
)

var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02",
	"20060102",
	"2006",
}

type probeResult struct {
	Duration    float64
	Title       string
	Description string
	Category    string
	Date        string // normalized YYYYMMDD, empty if absent/unparseable
	Season      int
	Episode     int
}

type prober interface {
	Probe(ctx context.Context, file string) (probeResult, error)
}

type ffprobeProber struct{}

func (ffprobeProber) Probe(ctx context.Context, file string) (probeResult, error) {
	// string_validation=ignore makes ffprobe pass non-UTF-8 tag bytes through verbatim;
	// the default (replace) substitutes them with U+FFFD before we could transcode.
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration:format_tags",
		"-of", "json=string_validation=ignore",
		file,
	)

	out, err := cmd.Output()
	if err != nil {
		return probeResult{}, err
	}

	return parseProbeOutput(out)
}

func parseProbeOutput(out []byte) (probeResult, error) {
	// Legacy containers carry Windows-1251 tags; transcode them up front, otherwise
	// json.Unmarshal replaces the non-UTF-8 bytes with U+FFFD.
	if !utf8.Valid(out) {
		out = decodeWindows1251(out)
	}

	var parsed struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return probeResult{}, err
	}

	dur, err := strconv.ParseFloat(strings.TrimSpace(parsed.Format.Duration), 64)
	if err != nil {
		return probeResult{}, err
	}

	tags := make(map[string]string, len(parsed.Format.Tags))
	for k, v := range parsed.Format.Tags {
		tags[strings.ToLower(k)] = strings.TrimSpace(v)
	}

	season, episode := parseEpisode(firstTag(tags, "episode_id", "episode_sort", "episode"))

	return probeResult{
		Duration:    dur,
		Title:       firstTag(tags, "title"),
		Description: firstTag(tags, "description", "synopsis", "summary", "comment"),
		Category:    firstTag(tags, "genre"),
		Date:        parseDate(firstTag(tags, "date", "year", "date_released")),
		Season:      season,
		Episode:     episode,
	}, nil
}

func decodeWindows1251(b []byte) []byte {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(charmap.Windows1251.DecodeByte(c))
	}
	return []byte(sb.String())
}

func firstTag(tags map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := tags[k]; v != "" {
			return v
		}
	}
	return ""
}

func parseDate(v string) string {
	if v == "" {
		return ""
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format("20060102")
		}
	}
	return ""
}

func parseEpisode(v string) (season, episode int) {
	for _, re := range seasonEpisodeRegexps {
		if m := re.FindStringSubmatch(v); m != nil {
			season, _ = strconv.Atoi(m[1])
			episode, _ = strconv.Atoi(m[2])
			return season, episode
		}
	}
	for _, re := range episodeOnlyRegexps {
		if m := re.FindStringSubmatch(v); m != nil {
			episode, _ = strconv.Atoi(m[1])
			return 0, episode
		}
	}
	// Bare episode number (e.g. Matroska episode_sort) with unknown season.
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		return 0, n
	}
	return 0, 0
}
