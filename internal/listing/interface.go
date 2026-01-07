package listing

import (
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
	Name() string
	Playlists() []string
	URLGenerator() *urlgen.Generator
	HTTPClient() HTTPClient
	Rules() []*channel.Rule
	ProxyConfig() proxy.Proxy
	IsProxied() bool
}

type EPG interface {
	Name() string
	EPGs() []string
	URLGenerator() *urlgen.Generator
	HTTPClient() HTTPClient
	ProxyConfig() proxy.Proxy
	IsProxied() bool
}
