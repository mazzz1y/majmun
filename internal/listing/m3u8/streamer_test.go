package m3u8

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"majmun/internal/app"
	"majmun/internal/config"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	configchannel "majmun/internal/config/rules/channel"
	"majmun/internal/hashid"
	"majmun/internal/listing"
	"majmun/internal/listing/m3u8/rules/channel"
	"majmun/internal/listing/m3u8/rules/playlist"
	"majmun/internal/urlgen"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
	"gopkg.in/yaml.v3"
)

func createStreamer(subscriptions []listing.Playlist, epgLink string) *Streamer {
	channelProcessor := channel.NewRulesProcessor("test", nil)
	playlistProcessor := playlist.NewRulesProcessor("test", nil)
	return &Streamer{
		subscriptions:     subscriptions,
		epgURL:            epgLink,
		channelProcessor:  channelProcessor,
		playlistProcessor: playlistProcessor,
	}
}

func createStreamerWithChannels(subscriptions []listing.Playlist, channels []listing.Channel, epgLink string) *Streamer {
	s := createStreamer(subscriptions, epgLink)
	s.channels = channels
	return s
}

type mockPlaylist struct {
	name   string
	urlGen *urlgen.Generator
}

func (m mockPlaylist) ID() string                      { return hashid.New(m.name) }
func (m mockPlaylist) Name() string                    { return m.name }
func (m mockPlaylist) Playlists() []string             { return nil }
func (m mockPlaylist) URLGenerator() *urlgen.Generator { return m.urlGen }
func (m mockPlaylist) HTTPClient() listing.HTTPClient  { return nil }
func (m mockPlaylist) Rules() []*configchannel.Rule    { return nil }
func (m mockPlaylist) ProxyConfig() proxy.Proxy        { return proxy.Proxy{} }
func (m mockPlaylist) IsProxied() bool                 { return false }
func (m mockPlaylist) SkipOnError() bool               { return false }

type mockChannel struct {
	name     string
	id       string
	playlist listing.Playlist
	logo     string
	fields   []config.ChannelField
	urlGen   *urlgen.Generator
}

func (m mockChannel) Name() string                    { return m.name }
func (m mockChannel) ID() string                      { return m.id }
func (m mockChannel) Playlist() listing.Playlist      { return m.playlist }
func (m mockChannel) Logo() string                    { return m.logo }
func (m mockChannel) Fields() []config.ChannelField   { return m.fields }
func (m mockChannel) URLGenerator() *urlgen.Generator { return m.urlGen }
func (m mockChannel) Programmes(_ context.Context, _ time.Time) ([]listing.Programme, error) {
	return nil, nil
}
func (m mockChannel) CatchupDays(_ time.Time) int { return 1 }

func channelField(t *testing.T, selectorRaw, templateStr string) config.ChannelField {
	t.Helper()
	var sel common.Selector
	require.NoError(t, yaml.Unmarshal([]byte(strconv.Quote(selectorRaw)), &sel))
	tmpl, err := template.New("t").Parse(templateStr)
	require.NoError(t, err)
	return config.ChannelField{Selector: &sel, Template: (*common.Template)(tmpl)}
}

func TestStreamerInjectsGeneratedChannel(t *testing.T) {
	ctx := context.Background()
	gen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	require.NoError(t, err)

	parent := mockPlaylist{name: "local", urlGen: gen}
	ch := mockChannel{
		name:     "Cartoons 24/7",
		id:       "abc123",
		playlist: parent,
		logo:     "http://example.com/l.png",
		fields: []config.ChannelField{
			channelField(t, "attr/group-title", "Kids"),
			channelField(t, "tag/EXTGRP", "Kids"),
		},
		urlGen: gen,
	}

	streamer := createStreamerWithChannels(nil, []listing.Channel{ch}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, `tvg-id="abc123"`)
	assert.Contains(t, output, "Cartoons 24/7")
	assert.Contains(t, output, `group-title="Kids"`)
	assert.Contains(t, output, "#EXTGRP:Kids")
	// the logo is wrapped in a signed majmun file URL, not emitted raw
	assert.NotContains(t, output, `tvg-logo="http://example.com/l.png"`)
	assert.Regexp(t, `tvg-logo="http://localhost/[^"]+/f\.png"`, output)

	// the stream token carries the display name (for logs/metrics) and routes by ID
	m := regexp.MustCompile(`http://localhost/([A-Za-z0-9_-]+)/f\.ts`).FindStringSubmatch(output)
	require.NotNil(t, m, "expected a stream URI in output:\n%s", output)
	data, err := gen.Decrypt(m[1])
	require.NoError(t, err)
	assert.Equal(t, "Cartoons 24/7", data.StreamData.ChannelName)
	require.Len(t, data.StreamData.Streams, 1)
	assert.Equal(t, "abc123", data.StreamData.Streams[0].ProviderInfo.ProviderID)
}

func TestStreamerChannelFieldsAttrOnly(t *testing.T) {
	ctx := context.Background()
	gen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	require.NoError(t, err)

	ch := mockChannel{
		name:     "Football",
		playlist: mockPlaylist{name: "local", urlGen: gen},
		fields:   []config.ChannelField{channelField(t, "attr/group-title", "1337 v2")},
		urlGen:   gen,
	}

	buffer := &bytes.Buffer{}
	_, err = createStreamerWithChannels(nil, []listing.Channel{ch}, "").WriteTo(ctx, buffer)
	require.NoError(t, err)

	out := buffer.String()
	assert.Contains(t, out, `group-title="1337 v2"`)
	assert.NotContains(t, out, "#EXTGRP")
}

func TestStreamerChannelFieldsTemplated(t *testing.T) {
	ctx := context.Background()
	gen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	require.NoError(t, err)

	ch := mockChannel{
		name:     "Football",
		playlist: mockPlaylist{name: "local", urlGen: gen},
		fields:   []config.ChannelField{channelField(t, "tag/EXTGRP", "{{ .Playlist.Name }}")},
		urlGen:   gen,
	}

	buffer := &bytes.Buffer{}
	_, err = createStreamerWithChannels(nil, []listing.Channel{ch}, "").WriteTo(ctx, buffer)
	require.NoError(t, err)

	out := buffer.String()
	assert.Contains(t, out, "#EXTGRP:local")
	assert.NotContains(t, out, "group-title")
}

func TestStreamerChannelRulesApplyToGeneratedChannel(t *testing.T) {
	ctx := context.Background()
	gen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	require.NoError(t, err)

	ch := mockChannel{name: "cartoons", playlist: mockPlaylist{name: "local", urlGen: gen}, urlGen: gen}

	tmpl, err := template.New("t").Parse("Comedy")
	require.NoError(t, err)
	rule := &configchannel.Rule{
		SetField: &configchannel.SetFieldRule{
			Selector: &common.Selector{Type: common.SelectorAttr, Value: "group-title"},
			Template: (*common.Template)(tmpl),
			Condition: &common.Condition{
				Playlists: common.StringOrArr{"local"},
			},
		},
	}

	channelProcessor := channel.NewRulesProcessor("test", []*configchannel.Rule{rule})
	streamer := &Streamer{
		channels:          []listing.Channel{ch},
		channelProcessor:  channelProcessor,
		playlistProcessor: playlist.NewRulesProcessor("test", nil),
	}

	buffer := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	assert.Contains(t, buffer.String(), `group-title="Comedy"`)
}

func TestStreamerChannelRulesSkipNonParentPlaylist(t *testing.T) {
	ctx := context.Background()
	gen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	require.NoError(t, err)

	ch := mockChannel{
		name:     "cartoons",
		playlist: mockPlaylist{name: "local", urlGen: gen},
		fields:   []config.ChannelField{channelField(t, "attr/group-title", "Kids")},
		urlGen:   gen,
	}

	tmpl, err := template.New("t").Parse("Comedy")
	require.NoError(t, err)
	rule := &configchannel.Rule{
		SetField: &configchannel.SetFieldRule{
			Selector: &common.Selector{Type: common.SelectorAttr, Value: "group-title"},
			Template: (*common.Template)(tmpl),
			Condition: &common.Condition{
				Playlists: common.StringOrArr{"other"},
			},
		},
	}

	streamer := &Streamer{
		channels:          []listing.Channel{ch},
		channelProcessor:  channel.NewRulesProcessor("test", []*configchannel.Rule{rule}),
		playlistProcessor: playlist.NewRulesProcessor("test", nil),
	}

	buffer := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	out := buffer.String()
	assert.Contains(t, out, `group-title="Kids"`)
	assert.NotContains(t, out, `group-title="Comedy"`)
}

type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func createTestSubscription(name string, playlists []string, httpClient listing.HTTPClient) (*app.Playlist, error) {
	sem := semaphore.NewWeighted(1)
	generator, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	if err != nil {
		return nil, err
	}
	return app.NewPlaylistProvider(
		name,
		generator,
		playlists,
		proxy.Proxy{},
		nil,
		sem,
		httpClient,
		false,
	)
}

func createTestSubscriptionWithSkip(name string, playlists []string, httpClient listing.HTTPClient) (*app.Playlist, error) {
	sem := semaphore.NewWeighted(1)
	generator, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	if err != nil {
		return nil, err
	}
	return app.NewPlaylistProvider(
		name,
		generator,
		playlists,
		proxy.Proxy{},
		nil,
		sem,
		httpClient,
		true,
	)
}

func TestStreamerWriteTo(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	sampleM3U := `#EXTM3U
#EXTINF:-1 tvg-id="test1" tvg-name="Test Channel 1" tvg-logo="http://example.com/logo.png" group-title="News", Test Channel 1
http://example.com/stream1
#EXTINF:0 tvg-id="test2" tvg-name="Test Channel 2", Test Channel 2
http://example.com/stream2`

	response := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(sampleM3U))),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist.m3u"
	})).Return(response, nil)

	sub, err := createTestSubscription(
		"test-subscription",
		[]string{"http://example.com/playlist.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{sub}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}

	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "#EXTM3U")
	assert.Contains(t, output, "x-tvg-url=\"http://example.com/epg.xml\"")
	assert.Contains(t, output, "Test Channel 1")
	assert.Contains(t, output, "Test Channel 2")
	assert.Contains(t, output, "http://example.com/stream1")
	assert.Contains(t, output, "http://example.com/stream2")

	httpClient.AssertExpectations(t)
}

func TestStreamerFilteringChannels(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	sampleM3U := `#EXTM3U
#EXTINF:-1 tvg-id="news1" tvg-name="News Channel 1" group-title="News", News Channel 1
http://example.com/news1
#EXTINF:-1 tvg-id="sports1" tvg-name="Sports Channel 1" group-title="Sports", Sports Channel 1
http://example.com/sports1
#EXTINF:-1 tvg-id="movies1" tvg-name="Movies Channel 1" group-title="Movies", Movies Channel 1
http://example.com/movies1`

	response := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(sampleM3U))),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist.m3u"
	})).Return(response, nil)

	sub, err := createTestSubscription(
		"test-subscription",
		[]string{"http://example.com/playlist.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{sub}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}

	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "News Channel 1")
	assert.Contains(t, output, "Sports Channel 1")
	assert.Contains(t, output, "Movies Channel 1")

	httpClient.AssertExpectations(t)
}

func TestStreamerDuplicateChannelRemoval(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	sampleM3U := `#EXTM3U
#EXTINF:-1 tvg-id="test1" tvg-name="Test Channel 1", Test Channel 1
http://example.com/stream1
#EXTINF:-1 tvg-id="test2" tvg-name="Test Channel 1", Test Channel 1
http://example.com/stream1_duplicate`

	response := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(sampleM3U))),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist.m3u"
	})).Return(response, nil)

	sub, err := createTestSubscription(
		"test-subscription",
		[]string{"http://example.com/playlist.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{sub}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}

	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	count := 0
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Test Channel 1") {
			count++
		}
	}

	assert.Equal(t, 2, count)

	httpClient.AssertExpectations(t)
}

func TestStreamerErrorHandling(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist.m3u"
	})).Return(nil, fmt.Errorf("connection failed"))

	sub, err := createTestSubscription(
		"test-subscription",
		[]string{"http://example.com/playlist.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{sub}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}

	_, err = streamer.WriteTo(ctx, buffer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection failed")

	httpClient.AssertExpectations(t)
}

func TestStreamerSkipsFailingSourceWhenSkipOnError(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	goodM3U := `#EXTM3U
#EXTINF:-1 tvg-id="ok1" tvg-name="OK Channel", OK Channel
http://example.com/ok`

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/bad.m3u"
	})).Return(nil, fmt.Errorf("connection failed"))

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/good.m3u"
	})).Return(&http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(goodM3U))),
	}, nil)

	badSub, err := createTestSubscriptionWithSkip(
		"bad-sub",
		[]string{"http://example.com/bad.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	goodSub, err := createTestSubscriptionWithSkip(
		"good-sub",
		[]string{"http://example.com/good.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{badSub, goodSub}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "OK Channel")

	httpClient.AssertExpectations(t)
}

func TestStreamerSkipOnErrorAllFailing(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/bad.m3u"
	})).Return(nil, fmt.Errorf("connection failed"))

	badSub, err := createTestSubscriptionWithSkip(
		"bad-sub",
		[]string{"http://example.com/bad.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{badSub}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buffer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no channels found")

	httpClient.AssertExpectations(t)
}

func TestStreamerWithMultipleSubscriptions(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	sampleM3U1 := `#EXTM3U
#EXTINF:-1 tvg-id="news1" group-title="News", News Channel 1
http://example.com/news1`

	sampleM3U2 := `#EXTM3U
#EXTINF:-1 tvg-id="sports1" group-title="Sports", Sports Channel 1
http://example.com/sports1`

	response1 := &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(sampleM3U1)))}
	response2 := &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(sampleM3U2)))}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist1.m3u"
	})).Return(response1, nil)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist2.m3u"
	})).Return(response2, nil)

	sub1, err := createTestSubscription(
		"subscription1",
		[]string{"http://example.com/playlist1.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	sub2, err := createTestSubscription(
		"subscription2",
		[]string{"http://example.com/playlist2.m3u"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{sub1, sub2}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}

	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "News Channel 1")
	assert.Contains(t, output, "Sports Channel 1")

	httpClient.AssertExpectations(t)
}

func TestStreamerWithMultipleSources(t *testing.T) {
	ctx := context.Background()
	httpClient := new(MockHTTPClient)

	sampleM3U1 := `#EXTM3U
#EXTINF:-1 tvg-id="news1" tvg-name="News Channel 1" group-title="News", News Channel 1
http://example.com/news1
#EXTINF:-1 tvg-id="sports1" tvg-name="Sports Channel 1" group-title="Sports", Sports Channel 1
http://example.com/sports1`

	sampleM3U2 := `#EXTM3U
#EXTINF:-1 tvg-id="movies1" tvg-name="Movies Channel 1" group-title="Movies", Movies Channel 1
http://example.com/movies1
#EXTINF:-1 tvg-id="music1" tvg-name="Music Channel 1" group-title="Music", Music Channel 1
http://example.com/music1`

	response1 := &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(sampleM3U1)))}
	response2 := &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(sampleM3U2)))}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist1.m3u"
	})).Return(response1, nil)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/playlist2.m3u"
	})).Return(response2, nil)

	sub, err := createTestSubscription(
		"test-subscription",
		[]string{
			"http://example.com/playlist1.m3u",
			"http://example.com/playlist2.m3u",
		},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.Playlist{sub}, "http://example.com/epg.xml")

	buffer := &bytes.Buffer{}

	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "News Channel 1")
	assert.Contains(t, output, "Sports Channel 1")
	assert.Contains(t, output, "Movies Channel 1")
	assert.Contains(t, output, "Music Channel 1")

	httpClient.AssertExpectations(t)
}
