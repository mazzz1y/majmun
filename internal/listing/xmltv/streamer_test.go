package xmltv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"majmun/internal/app"
	"majmun/internal/config"
	"majmun/internal/config/proxy"
	"majmun/internal/hashid"
	"majmun/internal/listing"
	"majmun/internal/urlgen"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func createStreamer(subscriptions []listing.EPG, channelIDToName map[string]string) *Streamer {
	return NewStreamer(subscriptions, nil, channelIDToName)
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

func createTestProvider(name string, epgs []string, httpClient listing.HTTPClient) (*app.EPG, error) {
	generator, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	if err != nil {
		return nil, err
	}
	return app.NewEPGProvider(
		name,
		generator,
		epgs,
		proxy.Proxy{},
		httpClient,
		false,
	)
}

func createTestProviderWithSkip(name string, epgs []string, httpClient listing.HTTPClient) (*app.EPG, error) {
	generator, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	if err != nil {
		return nil, err
	}
	return app.NewEPGProvider(
		name,
		generator,
		epgs,
		proxy.Proxy{},
		httpClient,
		true,
	)
}

func TestNewStreamer(t *testing.T) {
	var subscriptions []listing.EPG
	channels := map[string]string{"channel1": "Channel One"}

	streamer := createStreamer(subscriptions, channels)
	assert.NotNil(t, streamer)
	assert.Equal(t, channels, streamer.channelIDToName)
	assert.NotNil(t, streamer.addedProgrammes)
}

func TestStreamer_WriteTo(t *testing.T) {
	ctx := t.Context()
	streamer := createStreamer([]listing.EPG{}, nil)
	buf := bytes.NewBuffer(nil)
	_, err := streamer.WriteTo(ctx, buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no EPG sources found")

	httpClient := new(MockHTTPClient)

	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
	<display-name>Channel 1</display-name>
	<icon src="http://example.com/icon.png" width="100" height="100"/>
  </channel>
  <programme start="20230101120000 +0000" channel="channel1">
	<title>Test Programme</title>
	<desc>Programme description</desc>
	<icon src="http://example.com/prog.png" width="100" height="100"/>
  </programme>
</tv>`

	sub, err := createTestProvider("test-subscription", []string{"http://example.com/epg.xml"}, httpClient)
	require.NoError(t, err)

	response := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlContent)),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/epg.xml"
	})).Return(response, nil)

	channels := map[string]string{"channel1": "Channel One"}
	streamer = createStreamer([]listing.EPG{sub}, channels)
	buf = bytes.NewBuffer(nil)
	_, err = streamer.WriteTo(ctx, buf)
	require.NoError(t, err)
	result := buf.String()
	assert.NotEmpty(t, result)
	assert.Contains(t, strings.ToLower(result), "<channel id=\"channel1\">")
	assert.Contains(t, strings.ToLower(result), "<programme start=\"")
	assert.Contains(t, result, "http://localhost/")
	assert.Contains(t, result, "/f.png")
}

func TestStreamerSkipsFailingEPGWhenSkipOnError(t *testing.T) {
	ctx := t.Context()
	httpClient := new(MockHTTPClient)

	goodXML := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
	<display-name>Channel 1</display-name>
  </channel>
  <programme start="20230101120000 +0000" channel="channel1">
	<title>OK Programme</title>
  </programme>
</tv>`

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/bad.xml"
	})).Return(nil, fmt.Errorf("connection failed"))

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/good.xml"
	})).Return(&http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(goodXML)),
	}, nil)

	badSub, err := createTestProviderWithSkip(
		"bad-sub",
		[]string{"http://example.com/bad.xml"},
		httpClient,
	)
	require.NoError(t, err)

	goodSub, err := createTestProviderWithSkip(
		"good-sub",
		[]string{"http://example.com/good.xml"},
		httpClient,
	)
	require.NoError(t, err)

	channels := map[string]string{"channel1": "Channel One"}
	streamer := createStreamer([]listing.EPG{badSub, goodSub}, channels)

	buffer := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "<channel id=\"channel1\">")
	assert.Contains(t, output, "OK Programme")

	httpClient.AssertExpectations(t)
}

func TestStreamerEPGFailsWithoutSkip(t *testing.T) {
	ctx := t.Context()
	httpClient := new(MockHTTPClient)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/bad.xml"
	})).Return(nil, fmt.Errorf("connection failed"))

	badSub, err := createTestProvider(
		"bad-sub",
		[]string{"http://example.com/bad.xml"},
		httpClient,
	)
	require.NoError(t, err)

	streamer := createStreamer([]listing.EPG{badSub}, map[string]string{})

	buffer := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buffer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection failed")

	httpClient.AssertExpectations(t)
}

func TestStreamerWithMultipleEPGSources(t *testing.T) {
	ctx := t.Context()
	httpClient := new(MockHTTPClient)

	xmlContent1 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
	<display-name>Channel 1</display-name>
	<icon src="http://example.com/icon1.png"/>
  </channel>
  <programme start="20230101120000 +0000" channel="channel1">
	<title>Morning Show</title>
	<desc>A morning program</desc>
  </programme>
</tv>`

	xmlContent2 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel2">
	<display-name>Channel 2</display-name>
	<icon src="http://example.com/icon2.png"/>
  </channel>
  <programme start="20230101140000 +0000" channel="channel2">
	<title>Afternoon Show</title>
	<desc>An afternoon program</desc>
  </programme>
</tv>`

	response1 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlContent1)),
	}
	response2 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlContent2)),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/epg1.xml"
	})).Return(response1, nil)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/epg2.xml"
	})).Return(response2, nil)

	sub, err := createTestProvider(
		"test-subscription",
		[]string{
			"http://example.com/epg1.xml",
			"http://example.com/epg2.xml",
		},
		httpClient,
	)
	require.NoError(t, err)

	channels := map[string]string{
		"channel1": "Channel One",
		"channel2": "Channel Two",
	}

	streamer := createStreamer([]listing.EPG{sub}, channels)

	buffer := &bytes.Buffer{}

	n, err := streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)
	require.Greater(t, n, int64(0))

	output := buffer.String()

	assert.Contains(t, output, "<channel id=\"channel1\">")
	assert.Contains(t, output, "<title>Morning Show</title>")
	assert.Contains(t, output, "http://localhost/")
	assert.Contains(t, output, "<channel id=\"channel2\">")

	httpClient.AssertExpectations(t)
}

func TestStreamerWithMultipleSubscriptionsAndEPGs(t *testing.T) {
	ctx := t.Context()
	httpClient := new(MockHTTPClient)

	xmlContent1 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="news1">
	<display-name>News Channel</display-name>
  </channel>
</tv>`

	xmlContent2 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="movies1">
	<display-name>Movies Channel</display-name>
  </channel>
</tv>`

	response1 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlContent1)),
	}
	response2 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlContent2)),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/sub1_epg.xml"
	})).Return(response1, nil)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.Method == "GET" && req.URL.String() == "http://example.com/sub2_epg.xml"
	})).Return(response2, nil)

	sub1, err := createTestProvider(
		"subscription-1",
		[]string{"http://example.com/sub1_epg.xml"},
		httpClient,
	)
	require.NoError(t, err)

	sub2, err := createTestProvider(
		"subscription-2",
		[]string{"http://example.com/sub2_epg.xml"},
		httpClient,
	)
	require.NoError(t, err)

	channels := map[string]string{
		"news1":   "News Channel",
		"movies1": "Movies Channel",
	}

	streamer := createStreamer([]listing.EPG{sub1, sub2}, channels)

	buffer := &bytes.Buffer{}

	_, err = streamer.WriteTo(ctx, buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "<channel id=\"news1\">")
	assert.Contains(t, output, "<channel id=\"movies1\">")

	httpClient.AssertExpectations(t)
}

func TestChannelIDConflicts(t *testing.T) {
	ctx := t.Context()
	httpClient := new(MockHTTPClient)

	epg1 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="1337">
    <display-name>CNN</display-name>
  </channel>
  <programme start="20230101120000 +0000" channel="1337">
    <title>CNN News</title>
  </programme>
</tv>`

	epg2SameChannel := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="1337">
    <display-name>CNN</display-name>
  </channel>
  <programme start="20230101130000 +0000" channel="1337">
    <title>World News</title>
  </programme>
</tv>`

	epg3DifferentChannel := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="1337">
    <display-name>FOX News</display-name>
  </channel>
  <programme start="20230101140000 +0000" channel="1337">
    <title>FOX Report</title>
  </programme>
</tv>`

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/epg1.xml"
	})).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(epg1))}, nil)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/epg2.xml"
	})).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(epg2SameChannel))}, nil)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/epg3.xml"
	})).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(epg3DifferentChannel))}, nil)

	sub, err := createTestProvider("test", []string{
		"http://example.com/epg1.xml",
		"http://example.com/epg2.xml",
		"http://example.com/epg3.xml",
	}, httpClient)
	require.NoError(t, err)

	channels := map[string]string{"1337": "CNN Channel"}
	streamer := createStreamer([]listing.EPG{sub}, channels)

	buf := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buf)
	require.NoError(t, err)

	output := buf.String()

	assert.NotContains(t, output, "FOX Report")
	assert.NotContains(t, output, "FOX News")
	channelCount := strings.Count(output, "<channel id=")
	assert.Equal(t, 1, channelCount)

	httpClient.AssertExpectations(t)
}

func TestChannelNameMatching(t *testing.T) {
	ctx := t.Context()
	httpClient := new(MockHTTPClient)

	epg1 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="1337">
    <display-name>CNN</display-name>
  </channel>
  <programme start="20230101120000 +0000" channel="1337">
    <title>CNN News</title>
  </programme>
</tv>`

	epg2 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="9999">
    <display-name>CNN</display-name>
  </channel>
  <programme start="20230101130000 +0000" channel="9999">
    <title>World News</title>
  </programme>
</tv>`

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/epg1.xml"
	})).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(epg1))}, nil)

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/epg2.xml"
	})).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(epg2))}, nil)

	sub, err := createTestProvider("test", []string{
		"http://example.com/epg1.xml",
		"http://example.com/epg2.xml",
	}, httpClient)
	require.NoError(t, err)

	channels := map[string]string{hashid.New("CNN"): "CNN Channel"}
	streamer := createStreamer([]listing.EPG{sub}, channels)

	buf := &bytes.Buffer{}
	_, err = streamer.WriteTo(ctx, buf)
	require.NoError(t, err)

	output := buf.String()

	assert.Contains(t, output, "CNN News")
	assert.Contains(t, output, "World News")

	channelCount := strings.Count(output, "<channel id=")
	assert.Equal(t, 1, channelCount)

	httpClient.AssertExpectations(t)
}

func TestTVGIDConflictIssue(t *testing.T) {
	httpClient := new(MockHTTPClient)

	xmlData1 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
	<channel id="original1">
		<display-name>CNN</display-name>
	</channel>
	<programme start="20230101120000 +0000" stop="20230101130000 +0000" channel="original1">
		<title>News Hour</title>
	</programme>
</tv>`

	xmlData2 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
	<channel id="original2">
		<display-name>CNN</display-name>
	</channel>
	<programme start="20230101140000 +0000" stop="20230101150000 +0000" channel="original2">
		<title>Different Show</title>
	</programme>
</tv>`

	xmlData3 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
	<channel id="original3">
		<display-name>CNN</display-name>
	</channel>
	<programme start="20230101160000 +0000" stop="20230101170000 +0000" channel="original3">
		<title>Third Show</title>
	</programme>
</tv>`

	response1 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlData1)),
	}
	response2 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlData2)),
	}
	response3 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlData3)),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/source1"
	})).Return(response1, nil)
	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/source2"
	})).Return(response2, nil)
	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/source3"
	})).Return(response3, nil)

	sub1, err := createTestProvider("source1", []string{"http://example.com/source1"}, httpClient)
	require.NoError(t, err)
	sub2, err := createTestProvider("source2", []string{"http://example.com/source2"}, httpClient)
	require.NoError(t, err)
	sub3, err := createTestProvider("source3", []string{"http://example.com/source3"}, httpClient)
	require.NoError(t, err)

	expectedTVGID := hashid.New("CNN")
	channelMap := map[string]string{
		expectedTVGID: "CNN",
	}

	streamer := createStreamer([]listing.EPG{sub1, sub2, sub3}, channelMap)

	var buf bytes.Buffer
	_, err = streamer.WriteTo(t.Context(), &buf)
	require.NoError(t, err)

	output := buf.String()
	channelCount := strings.Count(output, "<channel id=")
	programmeCount := strings.Count(output, "<programme")

	assert.Equal(t, 1, channelCount, "Same-name channels should be merged into one")
	assert.Equal(t, 3, programmeCount, "All programmes should be preserved")
	assert.Contains(t, output, fmt.Sprintf(`<channel id="%s">`, expectedTVGID))

	httpClient.AssertExpectations(t)
}

func TestSameOriginalIDDifferentSources(t *testing.T) {
	httpClient := new(MockHTTPClient)

	xmlData1 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
	<channel id="ch1">
		<display-name>CNN US</display-name>
	</channel>
	<programme start="20230101120000 +0000" stop="20230101130000 +0000" channel="ch1">
		<title>US News</title>
	</programme>
</tv>`

	xmlData2 := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
	<channel id="ch1">
		<display-name>CNN International</display-name>
	</channel>
	<programme start="20230101140000 +0000" stop="20230101150000 +0000" channel="ch1">
		<title>World News</title>
	</programme>
</tv>`

	response1 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlData1)),
	}
	response2 := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(xmlData2)),
	}

	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/source1"
	})).Return(response1, nil)
	httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		return req.URL.String() == "http://example.com/source2"
	})).Return(response2, nil)

	sub1, err := createTestProvider("source1", []string{"http://example.com/source1"}, httpClient)
	require.NoError(t, err)
	sub2, err := createTestProvider("source2", []string{"http://example.com/source2"}, httpClient)
	require.NoError(t, err)

	tvgID1 := hashid.New("CNN US")
	tvgID2 := hashid.New("CNN International")
	channelMap := map[string]string{
		tvgID1: "CNN US",
		tvgID2: "CNN International",
	}

	streamer := createStreamer([]listing.EPG{sub1, sub2}, channelMap)

	var buf bytes.Buffer
	_, err = streamer.WriteTo(t.Context(), &buf)
	require.NoError(t, err)

	output := buf.String()
	channelCount := strings.Count(output, "<channel id=")
	programmeCount := strings.Count(output, "<programme")

	assert.Equal(t, 2, channelCount, "Different channels with same original ID from different sources should both be included")
	assert.Equal(t, 2, programmeCount, "All programmes from both channels should be included")
	assert.Contains(t, output, "CNN US")
	assert.Contains(t, output, "CNN International")

	httpClient.AssertExpectations(t)
}

type mockGenChannel struct {
	name   string
	id     string
	logo   string
	urlGen *urlgen.Generator
	progs  []listing.Programme
}

func (m mockGenChannel) Name() string                    { return m.name }
func (m mockGenChannel) ID() string                      { return m.id }
func (m mockGenChannel) Playlist() listing.Playlist      { return nil }
func (m mockGenChannel) Logo() string                    { return m.logo }
func (m mockGenChannel) Fields() []config.ChannelField   { return nil }
func (m mockGenChannel) URLGenerator() *urlgen.Generator { return m.urlGen }

func (m mockGenChannel) Programmes(_ context.Context, _ time.Time) ([]listing.Programme, error) {
	return m.progs, nil
}
func (m mockGenChannel) CatchupDays(_ time.Time) int { return 1 }

func TestStreamerEmitsGeneratedChannelEPG(t *testing.T) {
	start := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	gen, err := urlgen.NewGenerator("http://localhost", "secret", time.Hour, time.Hour)
	require.NoError(t, err)
	ch := mockGenChannel{
		name:   "Cartoons 24/7",
		id:     "abc123",
		logo:   "http://example.com/l.png",
		urlGen: gen,
		progs: []listing.Programme{
			{Title: "ep01", Description: "A short synopsis.", Category: "Animation", Date: "20210608", Season: 1, Episode: 3, Start: start, Stop: start.Add(time.Hour)},
			{Title: "ep02", Start: start.Add(time.Hour), Stop: start.Add(2 * time.Hour)},
		},
	}

	streamer := NewStreamer(nil, []listing.Channel{ch}, nil)

	var buf bytes.Buffer
	_, err = streamer.WriteTo(t.Context(), &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `<channel id="abc123">`)
	assert.Contains(t, output, "Cartoons 24/7")
	assert.NotContains(t, output, `src="http://example.com/l.png"`)
	assert.Regexp(t, `src="http://localhost/[^"]+/f\.png"`, output)
	assert.Contains(t, output, `channel="abc123"`)
	assert.Contains(t, output, "ep01")
	assert.Contains(t, output, "ep02")
	assert.Contains(t, output, "<desc>A short synopsis.</desc>")
	assert.Contains(t, output, "<category>Animation</category>")
	assert.Contains(t, output, "<date>20210608</date>")
	assert.Contains(t, output, `<episode-num system="xmltv_ns">0.2.</episode-num>`)
	assert.Contains(t, output, `<episode-num system="onscreen">S01E03</episode-num>`)
}
