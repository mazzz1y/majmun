package config

import (
	"majmun/internal/config/common"
	"testing"
	"time"
)

func TestPlayoutResolvedRefreshInterval(t *testing.T) {
	var p Playout
	if got := p.ResolvedRefreshInterval(); got != 0 {
		t.Errorf("expected 0 when unset, got %v", got)
	}
	tenMin := common.Duration(10 * time.Minute)
	p.RefreshInterval = &tenMin
	if got := p.ResolvedRefreshInterval(); got != 10*time.Minute {
		t.Errorf("expected 10m, got %v", got)
	}
}

func TestPlayoutResolvedScheduleSwapAt(t *testing.T) {
	var p Playout
	if h, m := p.ResolvedScheduleSwapAt(); h != 0 || m != 0 {
		t.Errorf("expected 0:0 when unset, got %02d:%02d", h, m)
	}
	p.ScheduleSwapAt = "23:45"
	if h, m := p.ResolvedScheduleSwapAt(); h != 23 || m != 45 {
		t.Errorf("expected 23:45, got %02d:%02d", h, m)
	}
}

func TestPlayoutValidateOrder(t *testing.T) {
	for _, ok := range []string{"", "sequential", "shuffle", "interleave"} {
		if err := (&Playout{Order: ok}).Validate(); err != nil {
			t.Errorf("order %q should be valid: %v", ok, err)
		}
	}
	if err := (&Playout{Order: "bogus"}).Validate(); err == nil {
		t.Error("expected error for invalid order")
	}
}

func TestPlayoutValidateFiller(t *testing.T) {
	valid := []PlayoutFiller{
		{},
		{Sources: common.StringOrArr{"/ads"}}, // sources alone -> default interval
		{Sources: common.StringOrArr{"/ads"}, EveryCount: 3},                     // count mode
		{Sources: common.StringOrArr{"/ads"}, Every: common.Duration(time.Hour)}, // time mode
		{Sources: common.StringOrArr{"/ads"}, EveryCount: 3, Order: "sequential"},
	}
	for i, a := range valid {
		if err := (&Playout{Filler: a}).Validate(); err != nil {
			t.Errorf("filler[%d] should be valid: %v", i, err)
		}
	}

	invalid := []PlayoutFiller{
		{EveryCount: 3},                     // cadence without sources
		{Every: common.Duration(time.Hour)}, // cadence without sources
		{Sources: common.StringOrArr{"/ads"}, EveryCount: 3, Every: common.Duration(time.Hour)}, // both modes
		{Sources: common.StringOrArr{"/ads"}, EveryCount: -1},                                   // negative count
		{Sources: common.StringOrArr{"/ads"}, EveryCount: 3, Order: "interleave"},               // unsupported order
	}
	for i, a := range invalid {
		if err := (&Playout{Filler: a}).Validate(); err == nil {
			t.Errorf("filler[%d] should be invalid", i)
		}
	}
}

func TestParsePlayoutSwapAt(t *testing.T) {
	valid := map[string][2]int{
		"00:00": {0, 0},
		"04:00": {4, 0},
		"23:59": {23, 59},
		" 9:05": {9, 5},
	}
	for in, want := range valid {
		h, m, err := ParsePlayoutSwapAt(in)
		if err != nil {
			t.Errorf("ParsePlayoutSwapAt(%q) unexpected error: %v", in, err)
			continue
		}
		if h != want[0] || m != want[1] {
			t.Errorf("ParsePlayoutSwapAt(%q) = %02d:%02d, want %02d:%02d", in, h, m, want[0], want[1])
		}
	}

	for _, in := range []string{"", "4", "4:00:00", "24:00", "04:60", "-1:00", "aa:bb", "04.00"} {
		if _, _, err := ParsePlayoutSwapAt(in); err == nil {
			t.Errorf("ParsePlayoutSwapAt(%q) expected error, got nil", in)
		}
	}
}

func TestPlayoutValidate(t *testing.T) {
	p := Playout{ScheduleSwapAt: "99:99"}
	if err := p.Validate(); err == nil {
		t.Error("expected error for invalid schedule_swap_at")
	}

	reserved := Playout{TemplateVars: []common.NameValue{{Name: "Playout", Value: "x"}}}
	if err := reserved.Validate(); err == nil {
		t.Error("expected error for reserved template variable")
	}

	initBad := Playout{InitSegments: -1}
	if err := initBad.Validate(); err == nil {
		t.Error("expected error for negative init_segments")
	}
}
