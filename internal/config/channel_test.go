package config

import (
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"net/url"
	"strconv"
	"testing"
	"text/template"

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

func validBaseConfig() Config {
	u, _ := url.Parse("http://example.com")
	return Config{
		Server:       ServerConfig{ListenAddr: ":8080", PublicURL: common.URL(*u)},
		Logs:         Logs{"info", "text"},
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
			channel:     Channel{Name: "cartoons", Sources: common.StringOrArr{"/media/cartoons"}, Playout: proxy.Playout{RandomOrder: boolPtr(true)}},
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
			name: "valid playout template variable",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				Playout: proxy.Playout{TemplateVars: []common.NameValue{{Name: "logo_width_pct", Value: "0.06"}}}},
			expectError: false,
		},
		{
			name: "invalid playout propagates",
			channel: Channel{Name: "c", Sources: common.StringOrArr{"/m"},
				Playout: proxy.Playout{TemplateVars: []common.NameValue{{Name: "input", Value: "x"}}}},
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
