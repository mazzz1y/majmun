package app

import (
	"majmun/internal/config/proxy"
	"majmun/internal/listing"
	"majmun/internal/shell"
	"majmun/internal/urlgen"
)

type EPG struct {
	name         string
	sources      []string
	urlGenerator *urlgen.Generator
	proxyConfig  proxy.Proxy
	httpClient   listing.HTTPClient
}

func NewEPGProvider(
	name string, urlGen *urlgen.Generator, sources []string, proxy proxy.Proxy,
	httpClient listing.HTTPClient) (*EPG, error) {
	return &EPG{
		name:         name,
		urlGenerator: urlGen,
		sources:      sources,
		proxyConfig:  proxy,
		httpClient:   httpClient,
	}, nil
}

func (es *EPG) Name() string {
	return es.name
}

func (es *EPG) Type() string {
	return "epg"
}

func (es *EPG) EPGs() []string {
	return es.sources
}

func (es *EPG) URLGenerator() *urlgen.Generator {
	return es.urlGenerator
}

func (es *EPG) HTTPClient() listing.HTTPClient {
	return es.httpClient
}

func (es *EPG) ProxyConfig() proxy.Proxy {
	return es.proxyConfig
}

func (es *EPG) IsProxied() bool {
	return es.proxyConfig.Enabled != nil && *es.proxyConfig.Enabled
}

func (es *EPG) ExpiredLinkStreamer() *shell.Streamer {
	return nil
}
