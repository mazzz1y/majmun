package proxy

import (
	"majmun/internal/config/common"
	"testing"
	"time"
)

func TestPlayoutResolvedRefreshInterval(t *testing.T) {
	var p Playout
	if got := p.ResolvedRefreshInterval(); got != defaultPlayoutRefresh {
		t.Errorf("expected default %v, got %v", defaultPlayoutRefresh, got)
	}

	zero := common.Duration(0)
	p.RefreshInterval = &zero
	if got := p.ResolvedRefreshInterval(); got != 0 {
		t.Errorf("expected 0 (disabled), got %v", got)
	}

	tenMin := common.Duration(10 * time.Minute)
	p.RefreshInterval = &tenMin
	if got := p.ResolvedRefreshInterval(); got != 10*time.Minute {
		t.Errorf("expected 10m, got %v", got)
	}
}

func TestPlayoutResolvedEPGDuration(t *testing.T) {
	var p Playout
	if got := p.ResolvedEPGDuration(); got != defaultPlayoutEPGDuration {
		t.Errorf("expected default %v, got %v", defaultPlayoutEPGDuration, got)
	}

	day := common.Duration(24 * time.Hour)
	p.EPGDuration = &day
	if got := p.ResolvedEPGDuration(); got != 24*time.Hour {
		t.Errorf("expected 24h, got %v", got)
	}
}

func TestPlayoutResolvedScheduleSwapAt(t *testing.T) {
	var p Playout
	if h, m := p.ResolvedScheduleSwapAt(); h != defaultPlayoutSwapHour || m != defaultPlayoutSwapMinute {
		t.Errorf("expected default %02d:%02d, got %02d:%02d", defaultPlayoutSwapHour, defaultPlayoutSwapMinute, h, m)
	}

	v := "23:45"
	p.ScheduleSwapAt = &v
	if h, m := p.ResolvedScheduleSwapAt(); h != 23 || m != 45 {
		t.Errorf("expected 23:45, got %02d:%02d", h, m)
	}
}

func TestPlayoutResolvedRandomOrder(t *testing.T) {
	var p Playout
	if p.ResolvedRandomOrder() {
		t.Error("expected false when unset")
	}
	tr := true
	p.RandomOrder = &tr
	if !p.ResolvedRandomOrder() {
		t.Error("expected true when set true")
	}
}

func TestPlayoutResolvedStateDir(t *testing.T) {
	var p Playout
	if got := p.ResolvedStateDir(); got != defaultPlayoutStateDir {
		t.Errorf("expected default %q, got %q", defaultPlayoutStateDir, got)
	}
	p.StateDir = "/var/lib/majmun"
	if got := p.ResolvedStateDir(); got != "/var/lib/majmun" {
		t.Errorf("expected override, got %q", got)
	}
}

func TestPlayoutResolvedExtensions(t *testing.T) {
	var p Playout
	if got := p.ResolvedExtensions(); len(got) != len(defaultPlayoutExtensions) {
		t.Errorf("expected defaults, got %v", got)
	}
	p.Extensions = common.StringOrArr{"mkv"}
	if got := p.ResolvedExtensions(); len(got) != 1 || got[0] != "mkv" {
		t.Errorf("expected override [mkv], got %v", got)
	}
}

func TestParseSwapAt(t *testing.T) {
	valid := map[string][2]int{
		"00:00": {0, 0},
		"04:00": {4, 0},
		"23:59": {23, 59},
		" 9:05": {9, 5},
	}
	for in, want := range valid {
		h, m, err := parseSwapAt(in)
		if err != nil {
			t.Errorf("parseSwapAt(%q) unexpected error: %v", in, err)
			continue
		}
		if h != want[0] || m != want[1] {
			t.Errorf("parseSwapAt(%q) = %02d:%02d, want %02d:%02d", in, h, m, want[0], want[1])
		}
	}

	for _, in := range []string{"", "4", "4:00:00", "24:00", "04:60", "-1:00", "aa:bb", "04.00"} {
		if _, _, err := parseSwapAt(in); err == nil {
			t.Errorf("parseSwapAt(%q) expected error, got nil", in)
		}
	}
}

func TestPlayoutValidate(t *testing.T) {
	bad := "99:99"
	p := Playout{ScheduleSwapAt: &bad}
	if err := p.Validate(); err == nil {
		t.Error("expected error for invalid schedule_swap_at")
	}

	reserved := Playout{TemplateVars: []common.NameValue{{Name: "input", Value: "x"}}}
	if err := reserved.Validate(); err == nil {
		t.Error("expected error for reserved template variable")
	}

	zero := 0
	initBad := Playout{InitSegments: &zero}
	if err := initBad.Validate(); err == nil {
		t.Error("expected error for init_segments < 1")
	}
}
