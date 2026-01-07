package app

import (
	"fmt"
	"majmun/internal/config"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	channelconf "majmun/internal/config/rules/channel"
	playlistconf "majmun/internal/config/rules/playlist"
	"majmun/internal/httpclient"
	"majmun/internal/listing"
	"majmun/internal/listing/m3u8/rules/channel"
	"majmun/internal/listing/m3u8/rules/playlist"
	"majmun/internal/shell"
	"majmun/internal/urlgen"

	"golang.org/x/sync/semaphore"
)

type Client struct {
	name              string
	secret            string
	semaphore         *semaphore.Weighted
	playlistProviders []*Playlist
	epgProviders      []*EPG
	proxy             proxy.Proxy
	channelProcessor  *channel.Processor
	playlistProcessor *playlist.Processor
	epgLink           string
	urlGen            *urlgen.Generator

	httpClientCfg common.HTTPClient
	cacheStore    *httpclient.Store
}

type Provider interface {
	Name() string
	Type() string
	URLGenerator() *urlgen.Generator
	ExpiredLinkStreamer() *shell.Streamer
	ProxyConfig() proxy.Proxy
	HTTPClient() listing.HTTPClient
}

func NewClient(
	clientCfg config.Client,
	urlGen *urlgen.Generator,
	channelRules []*channelconf.Rule,
	playlistRules []*playlistconf.Rule,
	publicURL string,
	cacheStore *httpclient.Store,
	httpClientCfg common.HTTPClient,
) (*Client, error) {
	if clientCfg.Secret == "" {
		return nil, fmt.Errorf("client secret cannot be empty")
	}

	var sem *semaphore.Weighted
	if clientCfg.Proxy.ConcurrentStreams > 0 {
		sem = semaphore.NewWeighted(clientCfg.Proxy.ConcurrentStreams)
	}

	return &Client{
		name:              clientCfg.Name,
		secret:            clientCfg.Secret,
		semaphore:         sem,
		proxy:             clientCfg.Proxy,
		channelProcessor:  channel.NewRulesProcessor(clientCfg.Name, channelRules),
		playlistProcessor: playlist.NewRulesProcessor(clientCfg.Name, playlistRules),
		epgLink:           fmt.Sprintf("%s/%s/epg.xml.gz", publicURL, clientCfg.Secret),
		urlGen:            urlGen,
		cacheStore:        cacheStore,
		httpClientCfg:     httpClientCfg,
	}, nil
}

func (c *Client) BuildPlaylistProvider(
	playlistConf config.Playlist,
	serverProxy proxy.Proxy,
	sem *semaphore.Weighted,
) error {
	mergedProxy := mergeProxies(serverProxy, playlistConf.Proxy, c.proxy)
	httpClient := c.newHTTPClient(mergedProxy)

	pr, err := NewPlaylistProvider(
		playlistConf.Name,
		c.urlGen,
		playlistConf.Sources,
		mergedProxy,
		nil,
		sem,
		httpClient,
	)
	if err != nil {
		return err
	}

	c.playlistProviders = append(c.playlistProviders, pr)
	return nil
}

func (c *Client) BuildEPGProvider(epgConf config.EPG, serverProxy proxy.Proxy) error {
	mergedProxy := mergeProxies(serverProxy, epgConf.Proxy, c.proxy)
	httpClient := c.newHTTPClient(mergedProxy)

	subscription, err := NewEPGProvider(
		epgConf.Name,
		c.urlGen,
		epgConf.Sources,
		mergedProxy,
		httpClient,
	)
	if err != nil {
		return err
	}

	c.epgProviders = append(c.epgProviders, subscription)
	return nil
}

func (c *Client) newHTTPClient(pr proxy.Proxy) listing.HTTPClient {
	opt := httpClientOptions(c.httpClientCfg, pr)
	if c.cacheStore == nil || !opt.CacheEnabled {
		return httpclient.NewDirectClient(opt.HTTPHeaders)
	}
	return c.cacheStore.NewHTTPClient(httpclient.Options{
		TTL:         opt.TTL,
		Retention:   opt.Retention,
		Compression: opt.Compression,
		HTTPHeaders: opt.HTTPHeaders,
	})
}

func (c *Client) PlaylistProviders() []listing.Playlist {
	result := make([]listing.Playlist, 0, len(c.playlistProviders))
	for _, ps := range c.playlistProviders {
		result = append(result, ps)
	}
	return result
}

func (c *Client) EPGProviders() []listing.EPG {
	result := make([]listing.EPG, 0, len(c.epgProviders))
	for _, es := range c.epgProviders {
		result = append(result, es)
	}
	return result
}

func (c *Client) GetProvider(prType urlgen.ProviderType, prName string) Provider {
	switch prType {
	case urlgen.ProviderTypePlaylist:
		for _, ps := range c.playlistProviders {
			if prName == ps.Name() {
				return ps
			}
		}
	case urlgen.ProviderTypeEPG:
		for _, ps := range c.epgProviders {
			if prName == ps.Name() {
				return ps
			}
		}
	}

	return nil
}

func (c *Client) Semaphore() *semaphore.Weighted {
	return c.semaphore
}

func (c *Client) EPGLink() string {
	return c.epgLink
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) URLGenerator() *urlgen.Generator {
	return c.urlGen
}

func (c *Client) ChannelProcessor() *channel.Processor {
	return c.channelProcessor
}

func (c *Client) PlaylistProcessor() *playlist.Processor {
	return c.playlistProcessor
}
