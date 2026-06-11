package channelgen

import (
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestParseProbeOutputUTF8(t *testing.T) {
	out := []byte(`{"format":{"duration":"60.0","tags":{"title":"Привет","genre":"Drama"}}}`)
	res, err := parseProbeOutput(out)
	if err != nil {
		t.Fatalf("parseProbeOutput: %v", err)
	}
	if res.Title != "Привет" {
		t.Errorf("title = %q, want %q", res.Title, "Привет")
	}
	if res.Category != "Drama" {
		t.Errorf("category = %q, want %q", res.Category, "Drama")
	}
}

func TestParseProbeOutputErrors(t *testing.T) {
	cases := map[string]string{
		"invalid json":       `not-json`,
		"missing duration":   `{"format":{"tags":{"title":"x"}}}`,
		"malformed duration": `{"format":{"duration":"abc"}}`,
	}
	for name, out := range cases {
		if _, err := parseProbeOutput([]byte(out)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseProbeOutputEpisode(t *testing.T) {
	cases := []struct {
		name            string
		tags            string
		season, episode int
	}{
		{"sXXeYY", `"episode_id":"S02E05"`, 2, 5},
		{"bare number", `"episode_sort":"7"`, 0, 7},
		{"no episode tags", `"title":"x"`, 0, 0},
	}
	for _, tc := range cases {
		out := []byte(`{"format":{"duration":"60.0","tags":{` + tc.tags + `}}}`)
		res, err := parseProbeOutput(out)
		if err != nil {
			t.Fatalf("%s: parseProbeOutput: %v", tc.name, err)
		}
		if res.Season != tc.season || res.Episode != tc.episode {
			t.Errorf("%s: season/episode = %d/%d, want %d/%d",
				tc.name, res.Season, res.Episode, tc.season, tc.episode)
		}
	}
}

func TestParseEpisodePatterns(t *testing.T) {
	cases := []struct {
		in              string
		season, episode int
	}{
		{"Show.S01E05.1080p", 1, 5},
		{"show s1.e5", 1, 5},
		{"Show 1x05", 1, 5},
		{"Show.01x05.WEB", 1, 5},
		{"Season 2 Episode 13", 2, 13},
		{"Show ep05", 0, 5},
		{"Show Episode 5", 0, 5},
		{"Show E12", 0, 12},
		{"Show e.3", 0, 3},
		{"7", 0, 7},
		{"  7  ", 0, 7},
		{"0", 0, 0},
		{"", 0, 0},
		{"Show 1280x720 x264", 0, 0}, // resolution/codec must not match
		{"Plain Movie Title", 0, 0},
	}
	for _, tc := range cases {
		season, episode := parseEpisode(tc.in)
		if season != tc.season || episode != tc.episode {
			t.Errorf("parseEpisode(%q) = %d/%d, want %d/%d", tc.in, season, episode, tc.season, tc.episode)
		}
	}
}

func TestParseProbeOutputWindows1251(t *testing.T) {
	// Build ffprobe-style JSON whose title bytes are Windows-1251 (not UTF-8), as emitted
	// for files muxed on legacy Windows systems. The raw bytes reach parseProbeOutput only
	// because Probe runs ffprobe with string_validation=ignore; the default writer mode
	// would have already replaced them with U+FFFD.
	title := "Кино"
	enc, err := charmap.Windows1251.NewEncoder().String(title)
	if err != nil {
		t.Fatalf("encode cp1251: %v", err)
	}
	out := []byte(`{"format":{"duration":"60.0","tags":{"title":"` + enc + `"}}}`)

	res, err := parseProbeOutput(out)
	if err != nil {
		t.Fatalf("parseProbeOutput: %v", err)
	}
	if res.Title != title {
		t.Errorf("title = %q, want %q (cp1251 not transcoded)", res.Title, title)
	}
}
