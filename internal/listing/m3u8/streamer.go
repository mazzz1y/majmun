package m3u8

import (
	"context"
	"fmt"
	"io"
	"majmun/internal/listing"
	"majmun/internal/listing/m3u8/rules/channel"
	"majmun/internal/listing/m3u8/rules/playlist"
	"majmun/internal/listing/m3u8/store"
	"majmun/internal/parser/m3u8"
)

type Streamer struct {
	subscriptions     []listing.Playlist
	epgURL            string
	channelProcessor  *channel.Processor
	playlistProcessor *playlist.Processor
}

func NewStreamer(subs []listing.Playlist, epgLink string, channelProcessor *channel.Processor, playlistProcessor *playlist.Processor) *Streamer {
	return &Streamer{
		subscriptions:     subs,
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

	processor := NewProcessor()

	return processor.Process(ctx, st, s.channelProcessor, s.playlistProcessor)
}

func (s *Streamer) fetchPlaylists(ctx context.Context) (*store.Store, error) {
	st := store.NewStore()

	var decoders []*decoderWrapper
	for _, sub := range s.subscriptions {
		for _, url := range sub.Playlists() {
			decoders = append(decoders, newDecoderWrapper(sub, sub.HTTPClient(), url))
		}
	}

	defer func() {
		for _, decoder := range decoders {
			if decoder != nil {
				_ = decoder.Close()
			}
		}
	}()

	for _, decoder := range decoders {
		err := decoder.StartBuffering(ctx)
		if err != nil {
			return nil, err
		}
	}

	for _, decoder := range decoders {
		if err := s.processTracks(ctx, decoder, st); err != nil {
			return nil, err
		}
	}

	if st.Len() == 0 {
		return nil, fmt.Errorf("no channels found in subscriptions")
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
