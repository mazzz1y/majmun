package app

import (
	"fmt"
	"majmun/internal/config/proxy"
	"majmun/internal/config/rules/channel"
	"majmun/internal/listing"
	"majmun/internal/shell"
	"majmun/internal/urlgen"

	"golang.org/x/sync/semaphore"
)

type Playlist struct {
	name string

	sources []string

	urlGenerator *urlgen.Generator
	semaphore    *semaphore.Weighted

	rules []*channel.Rule

	proxyConfig proxy.Proxy
	httpClient  listing.HTTPClient

	linkStreamer          *shell.Streamer
	rateLimitStreamer     *shell.Streamer
	upstreamErrorStreamer *shell.Streamer
	expiredLinkStreamer   *shell.Streamer
}

func NewPlaylistProvider(
	name string, urlGen *urlgen.Generator,
	sources []string,
	proxy proxy.Proxy, rules []*channel.Rule, sem *semaphore.Weighted,
	httpClient listing.HTTPClient) (*Playlist, error) {

	streamStreamer, err := shell.NewShellStreamer(
		proxy.Stream.Command,
		proxy.Stream.EnvVars,
		proxy.Stream.TemplateVars,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream command: %w", err)
	}

	rateLimitStreamer, err := shell.NewShellStreamer(
		proxy.Error.RateLimitExceeded.Command,
		proxy.Error.RateLimitExceeded.EnvVars,
		proxy.Error.RateLimitExceeded.TemplateVars,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limit command: %w", err)
	}

	upstreamErrorStreamer, err := shell.NewShellStreamer(
		proxy.Error.UpstreamError.Command,
		proxy.Error.UpstreamError.EnvVars,
		proxy.Error.UpstreamError.TemplateVars,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream error command: %w", err)
	}

	expiredLinkStreamer, err := shell.NewShellStreamer(
		proxy.Error.LinkExpired.Command,
		proxy.Error.LinkExpired.EnvVars,
		proxy.Error.LinkExpired.TemplateVars,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create expired link command: %w", err)
	}

	return &Playlist{
		name:                  name,
		urlGenerator:          urlGen,
		sources:               sources,
		semaphore:             sem,
		proxyConfig:           proxy,
		httpClient:            httpClient,
		rules:                 rules,
		linkStreamer:          streamStreamer,
		rateLimitStreamer:     rateLimitStreamer,
		upstreamErrorStreamer: upstreamErrorStreamer,
		expiredLinkStreamer:   expiredLinkStreamer,
	}, nil
}

func (ps *Playlist) Name() string {
	return ps.name
}

func (ps *Playlist) Type() string {
	return "playlist"
}

func (ps *Playlist) Playlists() []string {
	return ps.sources
}

func (ps *Playlist) URLGenerator() *urlgen.Generator {
	return ps.urlGenerator
}

func (ps *Playlist) HTTPClient() listing.HTTPClient {
	return ps.httpClient
}

func (ps *Playlist) Rules() []*channel.Rule {
	return ps.rules
}

func (ps *Playlist) Semaphore() *semaphore.Weighted {
	return ps.semaphore
}

func (ps *Playlist) IsProxied() bool {
	return ps.proxyConfig.Enabled != nil && *ps.proxyConfig.Enabled
}

func (ps *Playlist) ProxyConfig() proxy.Proxy {
	return ps.proxyConfig
}

func (ps *Playlist) LinkStreamer(streamUrl string) *shell.Streamer {
	return ps.linkStreamer.WithTemplateVars(map[string]any{"url": streamUrl})
}

func (ps *Playlist) LimitStreamer() *shell.Streamer {
	return ps.rateLimitStreamer
}

func (ps *Playlist) UpstreamErrorStreamer() *shell.Streamer {
	return ps.upstreamErrorStreamer
}

func (ps *Playlist) ExpiredLinkStreamer() *shell.Streamer {
	return ps.expiredLinkStreamer
}
