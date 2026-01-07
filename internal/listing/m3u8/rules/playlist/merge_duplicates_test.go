package playlist

import (
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"majmun/internal/config/rules/channel"
	configrules "majmun/internal/config/rules/playlist"
	"majmun/internal/listing"
	"majmun/internal/listing/m3u8/store"
	"majmun/internal/parser/m3u8"
	"majmun/internal/urlgen"
	"net/url"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

type mockPlaylist struct {
	name string
}

func (m mockPlaylist) Name() string                    { return m.name }
func (m mockPlaylist) Playlists() []string             { return nil }
func (m mockPlaylist) URLGenerator() *urlgen.Generator { return nil }
func (m mockPlaylist) Rules() []*channel.Rule          { return nil }

func (m mockPlaylist) HTTPClient() listing.HTTPClient { return nil }

func (m mockPlaylist) ProxyConfig() proxy.Proxy { return proxy.Proxy{} }
func (m mockPlaylist) IsProxied() bool          { return false }

func mustTemplate(tmpl string) *common.Template {
	var t common.Template
	node := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: tmpl,
	}
	if err := t.UnmarshalYAML(node); err != nil {
		panic(err)
	}
	return &t
}

func TestMergeChannelsProcessor_CopyTvgId(t *testing.T) {
	rule := &configrules.MergeDuplicatesRule{
		Selector: &common.Selector{Type: common.SelectorName},
		Patterns: common.RegexpArr{
			regexp.MustCompile(`4K`),
			regexp.MustCompile(`HD`),
		},
	}

	s := store.NewStore()

	playlist := mockPlaylist{name: "test-playlist"}

	uri1, _ := url.Parse("http://example.com/url1")
	uri2, _ := url.Parse("http://example.com/url2")

	track1 := &m3u8.Track{
		Name:  "CNN HD",
		URI:   uri1,
		Attrs: map[string]string{"tvg-id": "cnn-hd"},
	}
	track2 := &m3u8.Track{
		Name:  "CNN 4K",
		URI:   uri2,
		Attrs: map[string]string{"tvg-id": "cnn-4k"},
	}

	ch1 := store.NewChannel(track1, playlist)
	ch2 := store.NewChannel(track2, playlist)

	s.Add(ch1)
	s.Add(ch2)

	processor := &MergeDuplicatesProcessor{
		rule: rule,
		matcher: &mockDuplicatesMatcher{
			groups: map[string][]*store.Channel{
				"CNN": {ch2, ch1}, // ch2 is best
			},
		},
	}

	if err := processor.Apply(s); err != nil {
		t.Fatal(err)
	}

	channels := s.All()
	if len(channels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(channels))
		return
	}

	for _, ch := range channels {
		tvgID, exists := ch.GetAttr("tvg-id")
		if !exists {
			t.Errorf("Expected tvg-id attribute to exist")
			continue
		}
		if tvgID != "cnn-4k" {
			t.Errorf("Expected tvg-id 'cnn-4k', got '%s'", tvgID)
		}
	}
}

func TestMergeChannelsProcessor_SetFieldName(t *testing.T) {
	rule := &configrules.MergeDuplicatesRule{
		Selector: &common.Selector{Type: common.SelectorName},
		Patterns: common.RegexpArr{
			regexp.MustCompile(`4K`),
			regexp.MustCompile(`HD`),
		},
		FinalValue: &configrules.MergeDuplicatesFinalValue{
			Selector: &common.Selector{Type: common.SelectorName},
			Template: mustTemplate("{{.Channel.BaseName}} Multi-Quality"),
		},
	}

	processor := NewMergeDuplicatesActionProcessor(rule)
	s := store.NewStore()

	playlist := mockPlaylist{name: "test-playlist"}

	uri1, _ := url.Parse("http://example.com/url1")
	uri2, _ := url.Parse("http://example.com/url2")

	track1 := &m3u8.Track{
		Name:  "CNN HD",
		URI:   uri1,
		Attrs: map[string]string{"tvg-id": "cnn-hd"},
	}
	track2 := &m3u8.Track{
		Name:  "CNN 4K",
		URI:   uri2,
		Attrs: map[string]string{"tvg-id": "cnn-4k"},
	}

	ch1 := store.NewChannel(track1, playlist)
	ch2 := store.NewChannel(track2, playlist)

	s.Add(ch1)
	s.Add(ch2)

	if err := processor.Apply(s); err != nil {
		t.Fatal(err)
	}

	channels := s.All()
	for _, ch := range channels {
		if ch.Name() != "CNN Multi-Quality" {
			t.Errorf("Expected name 'CNN Multi-Quality', got '%s'", ch.Name())
		}
	}
}

func TestMergeChannelsProcessor_SetFieldAttr(t *testing.T) {
	rule := &configrules.MergeDuplicatesRule{
		Selector: &common.Selector{Type: common.SelectorName},
		Patterns: common.RegexpArr{
			regexp.MustCompile(`4K`),
			regexp.MustCompile(`HD`),
		},
		FinalValue: &configrules.MergeDuplicatesFinalValue{
			Selector: &common.Selector{Type: common.SelectorAttr, Value: "group-title"},
			Template: mustTemplate("{{.Channel.BaseName}} Group"),
		},
	}

	processor := NewMergeDuplicatesActionProcessor(rule)
	s := store.NewStore()

	playlist := mockPlaylist{name: "test-playlist"}

	uri1, _ := url.Parse("http://example.com/url1")
	uri2, _ := url.Parse("http://example.com/url2")

	track1 := &m3u8.Track{
		Name:  "CNN HD",
		URI:   uri1,
		Attrs: map[string]string{"tvg-id": "cnn-hd", "group-title": "News HD"},
	}
	track2 := &m3u8.Track{
		Name:  "CNN 4K",
		URI:   uri2,
		Attrs: map[string]string{"tvg-id": "cnn-4k", "group-title": "News 4K"},
	}

	ch1 := store.NewChannel(track1, playlist)
	ch2 := store.NewChannel(track2, playlist)

	s.Add(ch1)
	s.Add(ch2)

	if err := processor.Apply(s); err != nil {
		t.Fatal(err)
	}

	channels := s.All()
	for _, ch := range channels {
		groupTitle, exists := ch.GetAttr("group-title")
		if !exists {
			t.Errorf("Expected group-title attribute to exist")
			continue
		}
		if groupTitle != "CNN Group" {
			t.Errorf("Expected group-title 'CNN Group', got '%s'", groupTitle)
		}

		tvgID, exists := ch.GetAttr("tvg-id")
		if !exists {
			t.Errorf("Expected tvg-id attribute to exist")
			continue
		}
		if tvgID != "cnn-4k" {
			t.Errorf("Expected tvg-id 'cnn-4k', got '%s'", tvgID)
		}
	}
}

func TestMergeChannelsProcessor_SetFieldTag(t *testing.T) {
	rule := &configrules.MergeDuplicatesRule{
		Selector: &common.Selector{Type: common.SelectorName},
		Patterns: common.RegexpArr{
			regexp.MustCompile(`4K`),
			regexp.MustCompile(`HD`),
		},
		FinalValue: &configrules.MergeDuplicatesFinalValue{
			Selector: &common.Selector{Type: common.SelectorTag, Value: "quality"},
			Template: mustTemplate("{{.Channel.BaseName}} Multi"),
		},
	}

	processor := NewMergeDuplicatesActionProcessor(rule)
	s := store.NewStore()

	playlist := mockPlaylist{name: "test-playlist"}

	uri1, _ := url.Parse("http://example.com/url1")
	uri2, _ := url.Parse("http://example.com/url2")

	track1 := &m3u8.Track{
		Name:  "CNN HD",
		URI:   uri1,
		Attrs: map[string]string{"tvg-id": "cnn-hd"},
		Tags:  map[string]string{"quality": "HD"},
	}
	track2 := &m3u8.Track{
		Name:  "CNN 4K",
		URI:   uri2,
		Attrs: map[string]string{"tvg-id": "cnn-4k"},
		Tags:  map[string]string{"quality": "4K"},
	}

	ch1 := store.NewChannel(track1, playlist)
	ch2 := store.NewChannel(track2, playlist)

	s.Add(ch1)
	s.Add(ch2)

	if err := processor.Apply(s); err != nil {
		t.Fatal(err)
	}

	channels := s.All()
	for _, ch := range channels {
		quality, exists := ch.Tags()["quality"]
		if !exists {
			t.Errorf("Expected quality tag to exist")
			continue
		}
		if quality != "CNN Multi" {
			t.Errorf("Expected quality tag 'CNN Multi', got '%s'", quality)
		}

		tvgID, exists := ch.GetAttr("tvg-id")
		if !exists {
			t.Errorf("Expected tvg-id attribute to exist")
			continue
		}
		if tvgID != "cnn-4k" {
			t.Errorf("Expected tvg-id 'cnn-4k', got '%s'", tvgID)
		}
	}
}
