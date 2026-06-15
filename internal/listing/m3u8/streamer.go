package m3u8

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"majmun/internal/config/common"
	"majmun/internal/listing"
	"majmun/internal/listing/m3u8/rules/channel"
	"majmun/internal/listing/m3u8/rules/playlist"
	"majmun/internal/listing/m3u8/store"
	"majmun/internal/logging"
	"majmun/internal/parser/m3u8"
	"majmun/internal/urlgen"
	"strconv"
	"time"
)

type Streamer struct {
	subscriptions     []listing.Playlist
	channels          []listing.Channel
	epgURL            string
	channelProcessor  *channel.Processor
	playlistProcessor *playlist.Processor
}

func NewStreamer(subs []listing.Playlist, channels []listing.Channel, epgLink string, channelProcessor *channel.Processor, playlistProcessor *playlist.Processor) *Streamer {
	return &Streamer{
		subscriptions:     subs,
		channels:          channels,
		epgURL:            epgLink,
		channelProcessor:  channelProcessor,
		playlistProcessor: playlistProcessor,
	}
}

func (s *Streamer) WriteTo(ctx context.Context, w io.Writer) (int64, error) {
	channels, err := s.getChannels(ctx)
	if err != nil {
		return 0, err
	}

	writer := NewWriter(s.epgURL)
	return writer.WriteChannels(channels, w)
}

func (s *Streamer) GetAllChannels(ctx context.Context) (map[string]string, error) {
	channels, err := s.getChannels(ctx)
	if err != nil {
		return nil, err
	}

	channelMap := make(map[string]string)
	for _, ch := range channels {
		if tvgID, exists := ch.GetAttr("tvg-id"); exists {
			channelMap[tvgID] = ch.Name()
		}
	}

	return channelMap, nil
}

func (s *Streamer) getChannels(ctx context.Context) ([]*store.Channel, error) {
	st, err := s.fetchPlaylists(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.injectChannels(st); err != nil {
		return nil, err
	}

	if st.Len() == 0 {
		return nil, fmt.Errorf("no channels found in subscriptions")
	}

	processor := NewProcessor()

	return processor.Process(ctx, st, s.channelProcessor, s.playlistProcessor)
}

func (s *Streamer) injectChannels(st *store.Store) error {
	for _, ch := range s.channels {
		track := &m3u8.Track{
			Length: -1,
			Name:   ch.Name(),
			Attrs:  make(map[string]string),
			Tags:   make(map[string]string),
		}
		track.Attrs[m3u8.AttrTvgID] = ch.ID()
		days := strconv.Itoa(ch.CatchupDays(time.Now()))
		track.Attrs[m3u8.AttrCatchup] = "default"
		track.Attrs[m3u8.AttrCatchupDays] = days
		track.Attrs[m3u8.AttrTvgRec] = days
		logo, err := listing.ChannelLogoURL(ch)
		if err != nil {
			return err
		}
		if logo != "" {
			track.Attrs[m3u8.AttrTvgLogo] = logo
		}
		if err := applyChannelFields(track, ch); err != nil {
			return err
		}

		uri, err := ch.URLGenerator().CreateStreamURL(ch.Name(), []urlgen.Stream{{
			ProviderInfo: urlgen.ProviderInfo{
				ProviderType: urlgen.ProviderTypeChannel,
				ProviderID:   ch.ID(),
			},
		}})
		if err != nil {
			return fmt.Errorf("creating stream url for channel %q: %w", ch.Name(), err)
		}
		track.URI = uri

		st.Add(store.NewGeneratedChannel(track, ch.Playlist()))
	}

	return nil
}

// applyChannelFields renders the channel's configured field templates with the same context
// shape as set_field rules, writing each result to the selected attr or tag.
func applyChannelFields(track *m3u8.Track, ch listing.Channel) error {
	if len(ch.Fields()) == 0 {
		return nil
	}

	tmplMap := map[string]any{
		"Channel": map[string]any{
			"Name":  ch.Name(),
			"Attrs": track.Attrs,
			"Tags":  track.Tags,
		},
		"Playlist": map[string]any{
			"Name":      ch.Playlist().Name(),
			"IsProxied": ch.Playlist().IsProxied(),
		},
	}

	for _, field := range ch.Fields() {
		var buf bytes.Buffer
		if err := field.Template.ToTemplate().Execute(&buf, tmplMap); err != nil {
			return fmt.Errorf("channel %q field %q: %w", ch.Name(), field.Selector.Raw, err)
		}
		switch field.Selector.Type {
		case common.SelectorAttr:
			track.Attrs[field.Selector.Value] = buf.String()
		case common.SelectorTag:
			track.Tags[field.Selector.Value] = buf.String()
		}
	}

	return nil
}

func (s *Streamer) fetchPlaylists(ctx context.Context) (*store.Store, error) {
	st := store.NewStore()

	var decoders []*decoderWrapper
	var sourceURLs []string
	for _, sub := range s.subscriptions {
		for _, url := range sub.Playlists() {
			decoders = append(decoders, newDecoderWrapper(sub, sub.HTTPClient(), url))
			sourceURLs = append(sourceURLs, url)
		}
	}

	defer func() {
		for _, decoder := range decoders {
			if decoder != nil {
				_ = decoder.Close()
			}
		}
	}()

	skipped := make([]bool, len(decoders))
	for i, decoder := range decoders {
		err := decoder.StartBuffering(ctx)
		if err != nil {
			if decoder.subscription.SkipOnError() {
				logging.Error(ctx, err, "playlist source failed, skipping",
					"provider", decoder.subscription.Name(),
					"source", sourceURLs[i])
				skipped[i] = true
				continue
			}
			return nil, err
		}
	}

	for i, decoder := range decoders {
		if skipped[i] {
			continue
		}
		if err := s.processTracks(ctx, decoder, st); err != nil {
			if decoder.subscription.SkipOnError() {
				logging.Error(ctx, err, "playlist source failed mid-stream, skipping",
					"provider", decoder.subscription.Name(),
					"source", sourceURLs[i])
				continue
			}
			return nil, err
		}
	}

	return st, nil
}

func (s *Streamer) processTracks(ctx context.Context, decoder *decoderWrapper, st *store.Store) error {
	decoder.StopBuffer()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			item, err := decoder.NextItem()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if track, ok := item.(*m3u8.Track); ok {
				ch := store.NewChannel(track, decoder.subscription)
				st.Add(ch)
			}
		}
	}
}
