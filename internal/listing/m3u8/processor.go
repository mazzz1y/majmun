package m3u8

import (
	"context"
	"fmt"
	"majmun/internal/listing/m3u8/rules/channel"
	"majmun/internal/listing/m3u8/rules/playlist"
	"majmun/internal/listing/m3u8/store"
	"majmun/internal/parser/m3u8"
	"majmun/internal/urlgen"
	"net/url"
	"strings"
)

type Processor struct {
	channelsByID   map[string]*store.Channel
	channelsByName map[string]*store.Channel
	channelStreams map[*store.Channel][]urlgen.Stream
}

func NewProcessor() *Processor {
	return &Processor{
		channelsByID:   make(map[string]*store.Channel),
		channelsByName: make(map[string]*store.Channel),
		channelStreams: make(map[*store.Channel][]urlgen.Stream),
	}
}

func (p *Processor) Process(
	ctx context.Context,
	st *store.Store,
	channelProcessor *channel.Processor,
	playlistProcessor *playlist.Processor) ([]*store.Channel, error) {

	if err := channelProcessor.Apply(ctx, st); err != nil {
		return nil, err
	}
	if err := playlistProcessor.Apply(ctx, st); err != nil {
		return nil, err
	}

	for _, ch := range st.All() {
		if ch.IsRemoved() {
			continue
		}

		if existingChannel := p.findDuplicate(ch); existingChannel != nil {
			p.addStreamToChannel(existingChannel, ch)
			continue
		}

		if ch.Playlist().IsProxied() {
			if err := p.proxyChannelAttributes(ch); err != nil {
				return nil, err
			}
		}

		if ch.URI() != nil {
			p.channelStreams[ch] = []urlgen.Stream{p.createStream(ch)}
		} else {
			p.channelStreams[ch] = nil
		}

		p.trackChannel(ch)
	}

	for ch := range p.channelStreams {
		if ch.Playlist().IsProxied() && len(p.channelStreams[ch]) > 0 {
			p.updateChannelURI(ch)
		}
	}

	return p.collectResults(st), nil
}

func (p *Processor) findDuplicate(ch *store.Channel) *store.Channel {
	id, hasID := ch.GetAttr(m3u8.AttrTvgID)
	if hasID && id != "" {
		if existing, exists := p.channelsByID[id]; exists {
			return existing
		}
	} else {
		trackName := strings.ToLower(ch.Name())
		if existing, exists := p.channelsByName[trackName]; exists {
			return existing
		}
	}
	return nil
}

func (p *Processor) trackChannel(ch *store.Channel) {
	id, hasID := ch.GetAttr(m3u8.AttrTvgID)
	if hasID && id != "" {
		p.channelsByID[id] = ch
	} else {
		trackName := strings.ToLower(ch.Name())
		p.channelsByName[trackName] = ch
	}
}

func (p *Processor) createStream(ch *store.Channel) urlgen.Stream {
	return urlgen.Stream{
		ProviderInfo: urlgen.ProviderInfo{
			ProviderType: urlgen.ProviderTypePlaylist,
			ProviderName: ch.Playlist().Name(),
		},
		URL:    ch.URI().String(),
		Hidden: ch.IsHidden(),
	}
}

func (p *Processor) addStreamToChannel(existingChannel, newChannel *store.Channel) {
	if newChannel.URI() == nil {
		return
	}

	if newChannel.Priority() > existingChannel.Priority() {
		existingStreams := p.channelStreams[existingChannel]

		newStreamList := make([]urlgen.Stream, 0, 1+len(existingStreams))
		newStreamList = append(newStreamList, p.createStream(newChannel))
		newStreamList = append(newStreamList, existingStreams...)

		p.channelStreams[newChannel] = newStreamList
		delete(p.channelStreams, existingChannel)
		p.trackChannel(newChannel)
	} else {
		p.channelStreams[existingChannel] = append(
			p.channelStreams[existingChannel],
			p.createStream(newChannel),
		)
	}
}

func (p *Processor) proxyChannelAttributes(ch *store.Channel) error {
	urlGen := ch.Playlist().URLGenerator()
	providerInfo := urlgen.ProviderInfo{
		ProviderType: urlgen.ProviderTypePlaylist,
		ProviderName: ch.Playlist().Name(),
	}

	for key, value := range ch.Attrs() {
		if isURL(value) {
			u, err := urlGen.CreateFileURL(providerInfo, value)
			if err != nil {
				return fmt.Errorf("failed to encode attribute URL: %w", err)
			}
			ch.SetAttr(key, u.String())
		}
	}

	return nil
}

func (p *Processor) updateChannelURI(ch *store.Channel) {
	streams := p.channelStreams[ch]
	if len(streams) == 0 {
		return
	}

	urlGen := ch.Playlist().URLGenerator()
	if u, err := urlGen.CreateStreamURL(ch.Name(), streams); err == nil {
		ch.SetURI(u)
	}
}

func (p *Processor) collectResults(st *store.Store) []*store.Channel {
	result := make([]*store.Channel, 0, len(p.channelStreams))
	for _, ch := range st.All() {
		if _, exists := p.channelStreams[ch]; exists {
			result = append(result, ch)
		}
	}
	return result
}

func isURL(str string) bool {
	if str == "" {
		return false
	}

	u, err := url.Parse(str)
	return err == nil && u.Host != ""
}
