package shell

import (
	"slices"
	"testing"
)

func TestStreamerRendersNestedTemplateVars(t *testing.T) {
	base, err := NewShellStreamer([]string{"ffmpeg", "-i", "{{ .Channel.Logo }}", "{{ .Playout.Input }}"}, nil, nil)
	if err != nil {
		t.Fatalf("NewShellStreamer: %v", err)
	}

	s := base.WithTemplateVars(map[string]any{
		"Playout": map[string]any{"Input": "/tmp/list.txt"},
		"Channel": map[string]any{
			"Name": "Cartoons 24/7",
			"Logo": "/config/logos/cartoons.png",
		},
	})

	got, err := s.renderCommand(s.tmplVars)
	if err != nil {
		t.Fatalf("renderCommand: %v", err)
	}
	want := []string{"ffmpeg", "-i", "/config/logos/cartoons.png", "/tmp/list.txt"}
	if len(got) != len(want) {
		t.Fatalf("rendered %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEnvName(t *testing.T) {
	cases := map[string]string{
		"PlaylistPath": "PLAYLIST_PATH",
		"SegmentPath":  "SEGMENT_PATH",
		"Input":        "INPUT",
		"URL":          "URL",
		"Name":         "NAME",
		"Stream":       "STREAM",
	}
	for in, want := range cases {
		if got := envName(in); got != want {
			t.Errorf("envName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithTemplateVarsExposesEnv(t *testing.T) {
	base, err := NewShellStreamer([]string{"ffmpeg"}, nil, nil)
	if err != nil {
		t.Fatalf("NewShellStreamer: %v", err)
	}

	s := base.WithTemplateVars(map[string]any{
		"Stream": map[string]any{
			"URL":          "",
			"PlaylistPath": "/tmp/stream.m3u8",
		},
		"Playout": map[string]any{"Input": "/tmp/list.txt"},
	})

	want := []string{
		"MAJMUN_PLAYOUT_INPUT=/tmp/list.txt",
		"MAJMUN_STREAM_PLAYLIST_PATH=/tmp/stream.m3u8",
	}
	for _, w := range want {
		if !slices.Contains(s.envVars, w) {
			t.Errorf("env %q not found in %v", w, s.envVars)
		}
	}
	for _, e := range s.envVars {
		if e == "MAJMUN_STREAM_URL=" {
			t.Errorf("empty value should not be exported: %q", e)
		}
	}
}
