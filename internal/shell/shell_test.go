package shell

import (
	"testing"
)

func TestStreamerRendersNestedTemplateVars(t *testing.T) {
	base, err := NewShellStreamer([]string{"ffmpeg", "-i", "{{ .Channel.Logo }}", "{{ .input }}"}, nil, nil)
	if err != nil {
		t.Fatalf("NewShellStreamer: %v", err)
	}

	s := base.WithTemplateVars(map[string]any{
		"input": "/tmp/list.txt",
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
