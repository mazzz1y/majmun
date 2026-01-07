package m3u8

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"majmun/internal/app"
	"majmun/internal/config/proxy"
	"majmun/internal/listing"
	"majmun/internal/listing/m3u8/rules/channel"
	"majmun/internal/listing/m3u8/rules/playlist"
	"majmun/internal/urlgen"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
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
