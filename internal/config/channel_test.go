package config

import (
	"majmun/internal/config/common"
	"net/url"
	"strconv"
	"testing"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

func channelField(t *testing.T, selectorRaw, templateStr string) ChannelField {
	t.Helper()
	var sel common.Selector
	if err := yaml.Unmarshal([]byte(strconv.Quote(selectorRaw)), &sel); err != nil {
		t.Fatalf("parse selector %q: %v", selectorRaw, err)
	}
	tmpl, err := template.New("t").Parse(templateStr)
	if err != nil {
		t.Fatalf("parse template %q: %v", templateStr, err)
	}
	return ChannelField{Selector: &sel, Template: (*common.Template)(tmpl)}
}

func TestChannelResolvedRefreshInterval(t *testing.T) {
	c := Channel{Name: "c", Sources: common.StringOrArr{"/m"}}
	if got := c.ResolvedRefreshInterval(); got != defaultChannelRefresh {
		t.Errorf("expected default %v, got %v", defaultChannelRefresh, got)
	}

	zero := common.Duration(0)
	c.RefreshInterval = &zero
	if got := c.ResolvedRefreshInterval(); got != 0 {
		t.Errorf("expected 0 (disabled), got %v", got)
	}

	tenMin := common.Duration(10 * time.Minute)
	c.RefreshInterval = &tenMin
	if got := c.ResolvedRefreshInterval(); got != 10*time.Minute {
		t.Errorf("expected 10m, got %v", got)
	}
}

func TestChannelResolvedEPGDuration(t *testing.T) {
	c := Channel{Name: "c", Sources: common.StringOrArr{"/m"}}
	if got := c.ResolvedEPGDuration(); got != defaultChannelEPGDuration {
		t.Errorf("expected default %v, got %v", defaultChannelEPGDuration, got)
	}

	day := common.Duration(24 * time.Hour)
	c.EPGDuration = &day
	if got := c.ResolvedEPGDuration(); got != 24*time.Hour {
		t.Errorf("expected 24h, got %v", got)
	}
}

func TestChannelResolvedScheduleSwapAt(t *testing.T) {
	c := Channel{Name: "c", Sources: common.StringOrArr{"/m"}}
	if h, m := c.ResolvedScheduleSwapAt(); h != defaultScheduleSwapHour || m != defaultScheduleSwapMinute {
		t.Errorf("expected default %02d:%02d, got %02d:%02d", defaultScheduleSwapHour, defaultScheduleSwapMinute, h, m)
	}

	v := "23:45"
	c.ScheduleSwapAt = &v
	if h, m := c.ResolvedScheduleSwapAt(); h != 23 || m != 45 {
		t.Errorf("expected 23:45, got %02d:%02d", h, m)
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

func validBaseConfig() Config {
	u, _ := url.Parse("http://example.com")
	return Config{
		Server:       ServerConfig{ListenAddr: ":8080", PublicURL: common.URL(*u)},
		Logs:         Logs{"info", "text"},
		StateDir:     "state",
		URLGenerator: URLGeneratorConfig{Secret: "test"},
	}
}

func TestChannelValidate(t *testing.T) {
	tests := []struct {
		name        string
		channel     Channel
		expectError bool
	}{
		{
			name:        "valid sequential",
			channel:     Channel{Name: "cartoons", Sources: common.StringOrArr{"/media/cartoons"}},
			expectError: false,
		},
		{
			name:        "valid random",
			channel:     Channel{Name: "cartoons", Sources: common.StringOrArr{"/media/cartoons"}, RandomOrder: true},
			expectError: false,
		},
		{
			name:        "missing name",
			channel:     Channel{Sources: common.StringOrArr{"/media/cartoons"}},
			expectError: true,
		},
		{
			name:        "missing sources",
			channel:     Channel{Name: "cartoons"},
			expectError: true,
		},
		{
			name:        "empty source",
			channel:     Channel{Name: "cartoons", Sources: common.StringOrArr{""}},
			expectError: true,
		},
		{
			name: "valid attr field",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				Fields: []ChannelField{channelField(t, "attr/group-title", "Kids")}},
			expectError: false,
		},
		{
			name: "valid tag field",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				Fields: []ChannelField{channelField(t, "tag/EXTGRP", "Kids")}},
			expectError: false,
		},
		{
			name: "field selector name is rejected",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				Fields: []ChannelField{channelField(t, "name", "x")}},
			expectError: true,
		},
		{
			name: "field without selector is rejected",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				Fields: []ChannelField{{Template: channelField(t, "attr/x", "y").Template}}},
			expectError: true,
		},
		{
			name: "field without template is rejected",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				Fields: []ChannelField{{Selector: channelField(t, "attr/x", "y").Selector}}},
			expectError: true,
		},
		{
			name: "valid template variable",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				TemplateVars: []common.NameValue{{Name: "logo_width_pct", Value: "0.06"}}},
			expectError: false,
		},
		{
			name: "reserved template variable Channel is rejected",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				TemplateVars: []common.NameValue{{Name: "Channel", Value: "x"}}},
			expectError: true,
		},
		{
			name: "reserved template variable input is rejected",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				TemplateVars: []common.NameValue{{Name: "input", Value: "x"}}},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.channel.Validate()
			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigChannelValidation(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(c *Config)
		expectError bool
	}{
		{
			name: "duplicate channel name within one playlist",
			mutate: func(c *Config) {
				c.Playlists = []Playlist{
					{Name: "p1", Channels: []Channel{
						{Name: "a", Sources: common.StringOrArr{"/m"}},
						{Name: "a", Sources: common.StringOrArr{"/m"}},
					}},
				}
			},
			expectError: true,
		},
		{
			name: "same channel name across playlists is allowed",
			mutate: func(c *Config) {
				c.Playlists = []Playlist{
					{Name: "p1", Channels: []Channel{{Name: "a", Sources: common.StringOrArr{"/m"}}}},
					{Name: "p2", Channels: []Channel{{Name: "a", Sources: common.StringOrArr{"/m"}}}},
				}
			},
			expectError: false,
		},
		{
			name: "channel name matching playlist name is allowed",
			mutate: func(c *Config) {
				c.Playlists = []Playlist{
					{Name: "a", Sources: common.StringOrArr{"http://x"}},
					{Name: "p2", Channels: []Channel{{Name: "a", Sources: common.StringOrArr{"/m"}}}},
				}
			},
			expectError: false,
		},
		{
			name: "playlist with only channels is valid",
			mutate: func(c *Config) {
				c.Playlists = []Playlist{{Name: "p1", Channels: []Channel{{Name: "a", Sources: common.StringOrArr{"/m"}}}}}
			},
			expectError: false,
		},
		{
			name: "playlist with neither sources nor channels is invalid",
			mutate: func(c *Config) {
				c.Playlists = []Playlist{{Name: "p1"}}
			},
			expectError: true,
		},
		{
			name: "state_dir required when channels present",
			mutate: func(c *Config) {
				c.StateDir = ""
				c.Playlists = []Playlist{{Name: "p1", Channels: []Channel{{Name: "a", Sources: common.StringOrArr{"/m"}}}}}
			},
			expectError: true,
		},
		{
			name: "client references unknown playlist",
			mutate: func(c *Config) {
				c.Clients = []Client{{Name: "c", Secret: "s", Playlists: common.StringOrArr{"missing"}}}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
