package app

import (
	"majmun/internal/channelgen"
	"majmun/internal/config"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"majmun/internal/hashid"
	"majmun/internal/httpclient"
	"majmun/internal/urlgen"
	"testing"
	"time"
)

func testChannelProxy() proxy.Proxy {
	return proxy.Proxy{
		Stream: proxy.Handler{
			Command: common.StringOrArr{"ffmpeg", "-i", "{{ .input }}", "-f", "mpegts", "pipe:1"},
		},
		Playout: proxy.Segmenter{
			Command: common.StringOrArr{"ffmpeg", "-i", "{{ .input }}", "-f", "hls", "{{ .playlist_path }}"},
		},
		Error: proxy.Error{
			UpstreamError: proxy.Handler{
				Command: common.StringOrArr{"ffmpeg", "-f", "lavfi", "-i", "color=black", "pipe:1"},
			},
		},
	}
}

func newChannelProvider(t *testing.T, parentPlaylist string, conf config.Channel) *Channel {
	t.Helper()
	urlGen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mergedProxy := testChannelProxy()
	gen := channelgen.NewChannel(
		parentPlaylist,
		conf.Name,
		conf.Sources,
		conf.ResolvedExtensions(),
		conf.RandomOrder,
		conf.ResolvedRefreshInterval(),
		conf.ResolvedEPGDuration(),
		"state",
	)
	pl, err := NewPlaylistProvider(parentPlaylist, urlGen, nil, mergedProxy, nil, nil, httpclient.NewDirectClient(nil), false)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannelProvider(pl, conf, urlGen, gen, httpclient.NewDirectClient(nil), mergedProxy)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func TestChannelProviderDefaults(t *testing.T) {
	ch := newChannelProvider(t, "local", config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
	})

	if ch.Type() != "channel" {
		t.Errorf("expected type channel, got %s", ch.Type())
	}
	if ch.Name() != "cartoons" {
		t.Errorf("expected name to be channel name, got %s", ch.Name())
	}
	if ch.Playlist().Name() != "local" {
		t.Errorf("expected parent playlist 'local', got %s", ch.Playlist().Name())
	}
	if ch.ID() != hashid.New("local", "cartoons") {
		t.Errorf("expected id to hash the playlist and channel names, got %s", ch.ID())
	}
}

func TestChannelProviderSegmenterFromPlayout(t *testing.T) {
	ch := newChannelProvider(t, "local", config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
	})

	got := ch.Segmenter().Command
	if len(got) == 0 || got[0] != "ffmpeg" {
		t.Errorf("expected playout segmenter command, got %v", got)
	}
}

func TestChannelProviderTemplateVarsOverridePlayout(t *testing.T) {
	urlGen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mergedProxy := testChannelProxy()
	mergedProxy.Playout.TemplateVars = []common.NameValue{
		{Name: "logo_width_pct", Value: "0.06"},
		{Name: "fps", Value: "30"},
	}
	conf := config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
		TemplateVars: []common.NameValue{
			{Name: "logo_width_pct", Value: "0.10"},
			{Name: "custom", Value: "x"},
		},
	}
	pl, err := NewPlaylistProvider("local", urlGen, nil, mergedProxy, nil, nil, httpclient.NewDirectClient(nil), false)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannelProvider(pl, conf, urlGen, nil, httpclient.NewDirectClient(nil), mergedProxy)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, nv := range ch.Segmenter().TemplateVars {
		got[nv.Name] = nv.Value
	}
	want := map[string]string{"logo_width_pct": "0.10", "fps": "30", "custom": "x"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("template var %s = %q, want %q", k, got[k], v)
		}
	}
}

func TestChannelProviderClientStreamerInjectsInput(t *testing.T) {
	ch := newChannelProvider(t, "local", config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
	})
	if ch.ClientStreamer("/tmp/seg/stream.m3u8") == nil {
		t.Error("expected a client streamer")
	}
}
