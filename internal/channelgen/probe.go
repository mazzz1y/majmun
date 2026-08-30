package channelgen

import (
	"cmp"
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
	Show        string
	Description string
	Category    string
	Date        string // normalized YYYYMMDD, empty if absent/unparseable
	Season      int
	Episode     int
	EpisodeTag  string // raw episode_id/episode_sort/episode tag, parsed into Season/Episode by the builder

	// Media parameters of the first video/audio stream, for scripts to branch their
	// transcode on. All are empty/zero when the corresponding stream or field is absent.
	VideoCodec string
	Width      int
	Height     int
	// AspectWidth is the square-pixel display width (even), for a zero-copy scale_vaapi
	// to bake the aspect in. Equals Width when no correction applies; zero iff Width is.
	AspectWidth    int
	PixelFormat    string
	FrameRate      string // "30000/1001" form, as reported by ffprobe
	FieldOrder     string // progressive, tt, bb, tb, bt; empty if unknown
	AudioCodec     string
	AudioChannels  int
	SampleRate     int
	AudioLanguages []string // language tag per audio stream, in order; "und" when untagged
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
		"-show_entries",
		"format=duration:format_tags:stream_tags=language:"+
			"stream=codec_type,codec_name,width,height,sample_aspect_ratio,display_aspect_ratio,"+
			"pix_fmt,r_frame_rate,field_order,channels,sample_rate",
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
		Streams []struct {
			CodecType  string            `json:"codec_type"`
			CodecName  string            `json:"codec_name"`
			Width      int               `json:"width"`
			Height     int               `json:"height"`
			SAR        string            `json:"sample_aspect_ratio"`
			DAR        string            `json:"display_aspect_ratio"`
			PixFmt     string            `json:"pix_fmt"`
			RFrameRate string            `json:"r_frame_rate"`
			FieldOrder string            `json:"field_order"`
			Channels   int               `json:"channels"`
			SampleRate string            `json:"sample_rate"`
			Tags       map[string]string `json:"tags"`
		} `json:"streams"`
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return probeResult{}, err
	}

	var res probeResult
	var haveVideo, haveAudio bool
	for _, st := range parsed.Streams {
		switch st.CodecType {
		case "video":
			if haveVideo {
				continue
			}
			haveVideo = true
			res.VideoCodec = st.CodecName
			res.Width = st.Width
			res.Height = st.Height
			res.AspectWidth = aspectWidth(st.Width, st.Height, st.SAR, st.DAR)
			res.PixelFormat = st.PixFmt
			res.FrameRate = st.RFrameRate
			res.FieldOrder = st.FieldOrder
		case "audio":
			lang := cmp.Or(strings.TrimSpace(st.Tags["language"]), "und")
			res.AudioLanguages = append(res.AudioLanguages, lang)
			if haveAudio {
				continue
			}
			haveAudio = true
			res.AudioCodec = st.CodecName
			res.AudioChannels = st.Channels
			res.SampleRate, _ = strconv.Atoi(strings.TrimSpace(st.SampleRate))
		}
	}

	dur, err := strconv.ParseFloat(strings.TrimSpace(parsed.Format.Duration), 64)
	if err != nil {
		return probeResult{}, err
	}

	tags := make(map[string]string, len(parsed.Format.Tags))
	for k, v := range parsed.Format.Tags {
		tags[strings.ToLower(k)] = strings.TrimSpace(v)
	}

	res.Duration = dur
	res.Title = firstTag(tags, "title")
	res.Show = firstTag(tags, "show", "series")
	res.Description = firstTag(tags, "description", "synopsis", "summary", "comment")
	res.Category = firstTag(tags, "genre")
	res.Date = parseDate(firstTag(tags, "date", "year", "date_released"))
	res.EpisodeTag = firstTag(tags, "episode_id", "episode_sort", "episode")

	return res, nil
}

func decodeWindows1251(b []byte) []byte {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(charmap.Windows1251.DecodeByte(c))
	}
	return []byte(sb.String())
}

// aspectWidth returns the square-pixel display width, rounded even (encoders reject odd
// dimensions). It prefers the SAR (width*SAR), falls back to the DAR (height*DAR) for
// containers that report only it, and returns the width unchanged when neither is usable.
func aspectWidth(width, height int, sar, dar string) int {
	if width <= 0 {
		return 0
	}
	if num, den, ok := parseRatio(sar); ok {
		return roundEven((width*num + den/2) / den)
	}
	if num, den, ok := parseRatio(dar); ok && height > 0 {
		return roundEven((height*num + den/2) / den)
	}
	return roundEven(width)
}

// parseRatio parses an ffprobe "N:M" ratio, with ok=false for absent, malformed, or
// degenerate ("0:1", "1:1") values where no correction applies.
func parseRatio(s string) (num, den int, ok bool) {
	n, d, found := strings.Cut(s, ":")
	if !found {
		return 0, 0, false
	}
	pn, errN := strconv.Atoi(strings.TrimSpace(n))
	pd, errD := strconv.Atoi(strings.TrimSpace(d))
	if errN != nil || errD != nil || pn <= 0 || pd <= 0 || pn == pd {
		return 0, 0, false
	}
	return pn, pd, true
}

func roundEven(n int) int {
	return (n + 1) / 2 * 2
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

// parseEpisode extracts season and episode from v using episodePatterns. A pattern with two
// capture groups yields (season, episode); one group yields (0, episode). The first matching
// pattern wins, so order them most-specific first.
func parseEpisode(v string, episodePatterns []*regexp.Regexp) (season, episode int) {
	for _, re := range episodePatterns {
		m := re.FindStringSubmatch(v)
		if m == nil {
			continue
		}
		if len(m) >= 3 && m[1] != "" && m[2] != "" {
			season, _ = strconv.Atoi(m[1])
			episode, _ = strconv.Atoi(m[2])
			return season, episode
		}
		if len(m) >= 2 && m[1] != "" {
			episode, _ = strconv.Atoi(m[1])
			return 0, episode
		}
	}
	return 0, 0
}
