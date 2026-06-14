package channelgen

import (
	"reflect"
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

func TestParseProbeOutputVideoCodec(t *testing.T) {
	out := []byte(`{"streams":[{"codec_type":"audio","codec_name":"aac"},` +
		`{"codec_type":"video","codec_name":"hevc"}],"format":{"duration":"60.0"}}`)
	res, err := parseProbeOutput(out)
	if err != nil {
		t.Fatalf("parseProbeOutput: %v", err)
	}
	if res.VideoCodec != "hevc" {
		t.Errorf("video codec = %q, want %q", res.VideoCodec, "hevc")
	}

	out = []byte(`{"streams":[{"codec_type":"audio","codec_name":"mp3"}],"format":{"duration":"60.0"}}`)
	res, err = parseProbeOutput(out)
	if err != nil {
		t.Fatalf("parseProbeOutput: %v", err)
	}
	if res.VideoCodec != "" {
		t.Errorf("video codec = %q, want empty", res.VideoCodec)
	}
}

func TestParseProbeOutputMediaParams(t *testing.T) {
	out := []byte(`{"streams":[` +
		`{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,` +
		`"pix_fmt":"yuv420p","r_frame_rate":"30000/1001","field_order":"tt"},` +
		`{"codec_type":"audio","codec_name":"aac","channels":6,"sample_rate":"48000",` +
		`"tags":{"language":"eng"}}],` +
		`"format":{"duration":"60.0"}}`)
	res, err := parseProbeOutput(out)
	if err != nil {
		t.Fatalf("parseProbeOutput: %v", err)
	}

	want := probeResult{
		Duration: 60.0, VideoCodec: "h264", Width: 1920, Height: 1080,
		PixelFormat: "yuv420p", FrameRate: "30000/1001", FieldOrder: "tt",
		AudioCodec: "aac", AudioChannels: 6, SampleRate: 48000,
		AudioLanguages: []string{"eng"},
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("media params:\n got %+v\nwant %+v", res, want)
	}
}

func TestParseProbeOutputAudioLanguages(t *testing.T) {
	// Languages collected from every audio stream in order; untagged tracks become "und".
	out := []byte(`{"streams":[` +
		`{"codec_type":"video","codec_name":"h264"},` +
		`{"codec_type":"audio","codec_name":"aac","tags":{"language":"eng"}},` +
		`{"codec_type":"audio","codec_name":"ac3","tags":{"language":"rus"}},` +
		`{"codec_type":"audio","codec_name":"aac"}],` +
		`"format":{"duration":"60"}}`)
	res, err := parseProbeOutput(out)
	if err != nil {
		t.Fatalf("parseProbeOutput: %v", err)
	}
	want := []string{"eng", "rus", "und"}
	if !reflect.DeepEqual(res.AudioLanguages, want) {
		t.Errorf("audio languages = %v, want %v", res.AudioLanguages, want)
	}
}

func TestParseProbeOutputFirstStreamOfEachType(t *testing.T) {
	// Multiple video/audio streams: only the first of each kind is captured.
	out := []byte(`{"streams":[` +
		`{"codec_type":"video","codec_name":"hevc","width":3840,"height":2160},` +
		`{"codec_type":"video","codec_name":"mjpeg","width":320,"height":240},` +
		`{"codec_type":"audio","codec_name":"eac3","channels":8},` +
		`{"codec_type":"audio","codec_name":"aac","channels":2}],` +
		`"format":{"duration":"10"}}`)
	res, err := parseProbeOutput(out)
	if err != nil {
		t.Fatalf("parseProbeOutput: %v", err)
	}
	if res.VideoCodec != "hevc" || res.Width != 3840 || res.Height != 2160 {
		t.Errorf("video = %q %dx%d, want hevc 3840x2160", res.VideoCodec, res.Width, res.Height)
	}
	if res.AudioCodec != "eac3" || res.AudioChannels != 8 {
		t.Errorf("audio = %q %dch, want eac3 8ch", res.AudioCodec, res.AudioChannels)
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
