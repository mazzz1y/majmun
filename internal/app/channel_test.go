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
			Command: common.StringOrArr{"ffmpeg", "-i", "{{ .Stream.PlaylistPath }}", "-f", "mpegts", "pipe:1"},
		},
		Error: proxy.Error{
			UpstreamError: proxy.Handler{
				Command: common.StringOrArr{"ffmpeg", "-f", "lavfi", "-i", "color=black", "pipe:1"},
			},
		},
	}
}

func testPlayout() config.Playout {
	return config.Playout{
		Command: common.StringOrArr{"ffmpeg", "-i", "{{ .Playout.Input }}", "-f", "hls", "{{ .Stream.PlaylistPath }}"},
	}
}

func newChannelProvider(t *testing.T, parentPlaylist string, conf config.Channel, po config.Playout) *Channel {
	t.Helper()
	urlGen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mergedProxy := testChannelProxy()
	swapHour, swapMin := po.ResolvedScheduleSwapAt()
	gen := channelgen.NewChannel(channelgen.Config{
		Playlist:    parentPlaylist,
		Name:        conf.Name,
		Sources:     conf.Sources,
		Extensions:  po.Extensions,
		Order:       po.Order,
		Refresh:     po.ResolvedRefreshInterval(),
		EPGDuration: time.Duration(po.EPGDuration),
		SwapHour:    swapHour,
		SwapMin:     swapMin,
		StateDir:    "state",
	})
	pl, err := NewPlaylistProvider(parentPlaylist, urlGen, nil, mergedProxy, nil, nil, httpclient.NewDirectClient(nil), false)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannelProvider(pl, conf, po, urlGen, gen, httpclient.NewDirectClient(nil), mergedProxy)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func TestChannelProviderDefaults(t *testing.T) {
	ch := newChannelProvider(t, "local", config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
	}, testPlayout())

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

func TestChannelProviderPlayoutCommand(t *testing.T) {
	ch := newChannelProvider(t, "local", config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
	}, testPlayout())

	got := ch.Playout().Command
	if len(got) == 0 || got[0] != "ffmpeg" {
		t.Errorf("expected playout command, got %v", got)
	}
}

func TestChannelProviderLogoFromPlayout(t *testing.T) {
	playout := testPlayout()
	playout.Logo = "/config/logo.png"
	ch := newChannelProvider(t, "local", config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
	}, playout)

	if ch.Logo() != "/config/logo.png" {
		t.Errorf("expected logo from playout, got %q", ch.Logo())
	}
}

func TestChannelProviderClientStreamerInjectsInput(t *testing.T) {
	ch := newChannelProvider(t, "local", config.Channel{
		Name:    "cartoons",
		Sources: common.StringOrArr{"/media"},
	}, testPlayout())
	if ch.ClientStreamer("/tmp/seg/stream.m3u8") == nil {
		t.Error("expected a client streamer")
	}
}
