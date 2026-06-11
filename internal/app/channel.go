package app

import (
	"context"
	"fmt"
	"majmun/internal/channelgen"
	"majmun/internal/config"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"majmun/internal/hashid"
	"majmun/internal/listing"
	"majmun/internal/shell"
	"majmun/internal/streampool"
	"majmun/internal/urlgen"
	"time"
)

type Channel struct {
	name         string
	id           string
	playlist     listing.Playlist
	logo         string
	fields       []config.ChannelField
	urlGenerator *urlgen.Generator
	httpClient   listing.HTTPClient
	segmenter    proxy.Segmenter
	gen          *channelgen.Channel

	streamer              *shell.Streamer
	upstreamErrorStreamer *shell.Streamer
}

func NewChannelProvider(
	playlist listing.Playlist,
	channelConf config.Channel,
	urlGen *urlgen.Generator,
	gen *channelgen.Channel,
	httpClient listing.HTTPClient,
	mergedProxy proxy.Proxy,
) (*Channel, error) {
	streamStreamer, err := shell.NewShellStreamer(
		mergedProxy.Stream.Command,
		mergedProxy.Stream.EnvVars,
		mergedProxy.Stream.TemplateVars,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream command: %w", err)
	}

	upstreamErrorStreamer, err := shell.NewShellStreamer(
		mergedProxy.Error.UpstreamError.Command,
		mergedProxy.Error.UpstreamError.EnvVars,
		mergedProxy.Error.UpstreamError.TemplateVars,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream error command: %w", err)
	}

	playout := mergedProxy.Playout
	playout.TemplateVars = common.MergeNameValues(playout.TemplateVars, channelConf.TemplateVars)

	return &Channel{
		name:                  channelConf.Name,
		id:                    hashid.New(playlist.Name(), channelConf.Name),
		playlist:              playlist,
		logo:                  channelConf.Logo,
		fields:                channelConf.Fields,
		urlGenerator:          urlGen,
		httpClient:            httpClient,
		segmenter:             playout,
		gen:                   gen,
		streamer:              streamStreamer,
		upstreamErrorStreamer: upstreamErrorStreamer,
	}, nil
}

func (c *Channel) Name() string {
	return c.name
}

// ID is the channel's stable identity: hashid.New(playlist, name). It is the tvg-id, the
// EPG channel id, the token's provider ID, the schedule file name, and the stream key base,
// so it stays unique and URL-safe regardless of the human-readable name.
func (c *Channel) ID() string {
	return c.id
}

func (c *Channel) Playlist() listing.Playlist {
	return c.playlist
}

func (c *Channel) Type() string {
	return "channel"
}

func (c *Channel) Logo() string {
	return c.logo
}

func (c *Channel) Fields() []config.ChannelField {
	return c.fields
}

func (c *Channel) URLGenerator() *urlgen.Generator {
	return c.urlGenerator
}

func (c *Channel) HTTPClient() listing.HTTPClient {
	return c.httpClient
}

func (c *Channel) ProxyConfig() proxy.Proxy {
	return proxy.Proxy{}
}

func (c *Channel) ExpiredLinkStreamer() *shell.Streamer {
	return nil
}

func (c *Channel) Segmenter() proxy.Segmenter {
	return c.segmenter
}

func (c *Channel) Generator() *channelgen.Channel {
	return c.gen
}

func (c *Channel) Programmes(ctx context.Context, now time.Time) ([]listing.Programme, error) {
	progs, err := c.gen.Programmes(ctx, now)
	if err != nil {
		return nil, err
	}
	result := make([]listing.Programme, 0, len(progs))
	for _, p := range progs {
		result = append(result, listing.Programme{
			Title:       p.Title,
			Description: p.Description,
			Category:    p.Category,
			Date:        p.Date,
			Season:      p.Season,
			Episode:     p.Episode,
			Start:       p.Start,
			Stop:        p.Stop,
		})
	}
	return result, nil
}

func (c *Channel) ClientStreamer(playlistPath string) streampool.Streamer {
	return c.streamer.WithTemplateVars(map[string]any{"input": playlistPath})
}

func (c *Channel) UpstreamErrorStreamer() *shell.Streamer {
	return c.upstreamErrorStreamer
}
