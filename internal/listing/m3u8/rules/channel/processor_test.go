package channel

import (
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"majmun/internal/config/rules/channel"
	"majmun/internal/listing"
	"majmun/internal/listing/m3u8/store"
	"majmun/internal/parser/m3u8"
	"majmun/internal/urlgen"
	"net/url"
	"regexp"
	"testing"
)

func mustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

type mockPlaylist struct {
	name string
}

func (m mockPlaylist) Name() string                    { return m.name }
func (m mockPlaylist) Playlists() []string             { return nil }
func (m mockPlaylist) URLGenerator() *urlgen.Generator { return nil }
func (m mockPlaylist) Rules() []*channel.Rule          { return nil }
func (m mockPlaylist) ProxyConfig() proxy.Proxy        { return proxy.Proxy{} }

func (m mockPlaylist) HTTPClient() listing.HTTPClient { return nil }

func (m mockPlaylist) IsProxied() bool { return false }

func TestConditionLogic(t *testing.T) {
	playlist := mockPlaylist{name: "pl1"}
	processor := NewRulesProcessor("client1", nil)

	tests := []struct {
		name        string
		condition   common.Condition
		expectMatch bool
	}{
		{
			name: "client and tag both match",
			condition: common.Condition{
				Clients:  common.StringOrArr{"client1", "client2"},
				Selector: &common.Selector{Type: common.SelectorTag, Value: "cat"},
				Patterns: common.RegexpArr{mustCompile("restricted")},
			},
			expectMatch: true,
		},
		{
			name: "client matches but tag pattern doesn't",
			condition: common.Condition{
				Clients:  common.StringOrArr{"client1", "client2"},
				Selector: &common.Selector{Type: common.SelectorTag, Value: "cat"},
				Patterns: common.RegexpArr{mustCompile("safe")},
			},
			expectMatch: false,
		},
		{
			name: "client doesn't match",
			condition: common.Condition{
				Clients:  common.StringOrArr{"client3"},
				Selector: &common.Selector{Type: common.SelectorTag, Value: "cat"},
				Patterns: common.RegexpArr{mustCompile("restricted")},
			},
			expectMatch: false,
		},
		{
			name: "client matches with no other conditions",
			condition: common.Condition{
				Clients: common.StringOrArr{"client1", "client2"},
			},
			expectMatch: true,
		},
		{
			name: "no client condition, tag matches",
			condition: common.Condition{
				Selector: &common.Selector{Type: common.SelectorTag, Value: "cat"},
				Patterns: common.RegexpArr{mustCompile("restricted")},
			},
			expectMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, _ := url.Parse("http://example.com/stream")
			track := &m3u8.Track{
				Name: "Channel A",
				URI:  uri,
				Tags: map[string]string{"cat": "restricted"},
			}
			ch := store.NewChannel(track, playlist)

			result := processor.matchesCondition(ch, tt.condition)
			if result != tt.expectMatch {
				t.Errorf("matchesCondition() = %v, want %v", result, tt.expectMatch)
			}

			rule := &channel.RemoveChannelRule{Condition: &tt.condition}
			processor.processRemoveChannel(ch, rule)
			if ch.IsRemoved() != tt.expectMatch {
				t.Errorf("processRemoveChannel() = %v, want %v", ch.IsRemoved(), tt.expectMatch)
			}
		})
	}
}

func TestPlaylistCondition(t *testing.T) {
	playlist := mockPlaylist{name: "pl2"}
	uri, _ := url.Parse("http://example.com/stream")
	track := &m3u8.Track{Name: "Channel B", URI: uri}
	ch := store.NewChannel(track, playlist)
	processor := NewRulesProcessor("client1", nil)

	condition := common.Condition{
		Clients:   common.StringOrArr{"client1", "client2"},
		Playlists: common.StringOrArr{"pl2"},
	}

	result := processor.matchesCondition(ch, condition)
	if !result {
		t.Error("expected match when both client and playlist match")
	}

	condition.Playlists = common.StringOrArr{"pl3"}
	result = processor.matchesCondition(ch, condition)
	if result {
		t.Error("expected no match when playlist doesn't match")
	}
}

func TestAdultChannelFilteringWithClientAndOrConditions(t *testing.T) {
	playlist := mockPlaylist{name: "test-playlist"}
	uri, _ := url.Parse("http://example.com/stream")

	track := &m3u8.Track{
		Name: "NSFW Adult Channel",
		Tags: map[string]string{
			"EXTGRP": "adult",
		},
		URI: uri,
	}
	ch := store.NewChannel(track, playlist)

	restrictedProcessor := NewRulesProcessor("tv-bedroom", nil)
	condition := common.Condition{
		Clients: common.StringOrArr{"tv-bedroom", "tv2-bedroom"},
		Or: []common.Condition{
			{
				Selector: &common.Selector{Type: common.SelectorName},
				Patterns: common.RegexpArr{mustCompile(".*NSFW.*")},
			},
			{
				Selector: &common.Selector{Type: common.SelectorTag, Value: "EXTGRP"},
				Patterns: common.RegexpArr{mustCompile("(?i)adult")},
			},
		},
	}

	result := restrictedProcessor.matchesCondition(ch, condition)
	if !result {
		t.Error("expected match for restricted client with adult content")
	}

	allowedProcessor := NewRulesProcessor("living-room", nil)
	result = allowedProcessor.matchesCondition(ch, condition)
	if result {
		t.Error("expected no match for non-restricted client - adult content should be available")
	}

	restrictedProcessor2 := NewRulesProcessor("tv2-bedroom", nil)
	result = restrictedProcessor2.matchesCondition(ch, condition)
	if !result {
		t.Error("expected match for tv2-bedroom restricted client with adult content")
	}
}

func TestEvaluateFieldConditionEdgeCases(t *testing.T) {
	playlist := mockPlaylist{name: "test-playlist"}
	uri, _ := url.Parse("http://example.com/stream")
	track := &m3u8.Track{Name: "Test Channel", URI: uri}
	ch := store.NewChannel(track, playlist)
	processor := NewRulesProcessor("client1", nil)

	emptyCondition := common.Condition{}
	result := processor.evaluateField(ch, emptyCondition)
	if !result {
		t.Error("expected true for empty field condition")
	}

	conditionWithOnlyOr := common.Condition{
		Or: []common.Condition{
			{
				Selector: &common.Selector{Type: common.SelectorName},
				Patterns: common.RegexpArr{mustCompile("Test.*")},
			},
		},
	}
	result = processor.evaluateField(ch, conditionWithOnlyOr)
	if !result {
		t.Error("expected true when no field conditions are specified")
	}

	conditionMissingAttr := common.Condition{
		Selector: &common.Selector{Type: common.SelectorAttr, Value: "non-existent-attr"},
		Patterns: common.RegexpArr{mustCompile(".*")},
	}
	result = processor.evaluateField(ch, conditionMissingAttr)
	if result {
		t.Error("expected false for non-existent attribute")
	}

	conditionMissingTag := common.Condition{
		Selector: &common.Selector{Type: common.SelectorTag, Value: "non-existent-tag"},
		Patterns: common.RegexpArr{mustCompile(".*")},
	}
	result = processor.evaluateField(ch, conditionMissingTag)
	if result {
		t.Error("expected false for non-existent tag")
	}
}
