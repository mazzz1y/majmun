package listing

import (
	"context"
	"fmt"
	"majmun/internal/config"
	"majmun/internal/config/proxy"
	"majmun/internal/config/rules/channel"
	"majmun/internal/urlgen"
	"net/http"
	"net/url"
	"time"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Decoder interface {
	Decode() (any, error)
}

type URLGenerator interface {
	CreateURL(data urlgen.Data, ttl time.Duration) (*url.URL, error)
}

type Playlist interface {
	ID() string
	Name() string
	Playlists() []string
	URLGenerator() *urlgen.Generator
	HTTPClient() HTTPClient
	Rules() []*channel.Rule
	ProxyConfig() proxy.Proxy
	IsProxied() bool
	SkipOnError() bool
}

type EPG interface {
	ID() string
	Name() string
	EPGs() []string
	URLGenerator() *urlgen.Generator
	HTTPClient() HTTPClient
	ProxyConfig() proxy.Proxy
	IsProxied() bool
	SkipOnError() bool
}

type Programme struct {
	Title       string
	Description string
	Category    string
	Date        string
	Season      int
	Episode     int
	Start       time.Time
	Stop        time.Time
}

type Channel interface {
	ID() string
	Name() string
	Logo() string
	Fields() []config.ChannelField
	Playlist() Playlist
	URLGenerator() *urlgen.Generator
	Programmes(ctx context.Context, now time.Time) ([]Programme, error)
	CatchupDays(now time.Time) int
}

// ChannelLogoURL wraps the channel's logo in a signed file URL, or returns "" when no logo is set.
func ChannelLogoURL(ch Channel) (string, error) {
	if ch.Logo() == "" {
		return "", nil
	}
	logo, err := ch.URLGenerator().CreateFileURL(urlgen.ProviderInfo{
		ProviderType: urlgen.ProviderTypeChannel,
		ProviderID:   ch.ID(),
	}, ch.Logo())
	if err != nil {
		return "", fmt.Errorf("creating logo url for channel %q: %w", ch.Name(), err)
	}
	return logo.String(), nil
}
