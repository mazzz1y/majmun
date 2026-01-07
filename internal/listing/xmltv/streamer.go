package xmltv

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"majmun/internal/ioutil"
	"majmun/internal/listing"
	"majmun/internal/parser/xmltv"
	"majmun/internal/urlgen"
)

type Streamer struct {
	subscriptions    []listing.EPG
	channelIDToName  map[string]string
	addedChannels    map[string][]string
	addedProgrammes  map[string]bool
	channelIDMapping map[string]string
}

type Encoder interface {
	Encode(item any) error
	WriteFooter() error
	Close() error
}

func NewStreamer(subs []listing.EPG, channelIDToName map[string]string) *Streamer {
	subscriptions := subs
	channelLen := len(channelIDToName)
	approxProgrammeLen := 300 * channelLen

	return &Streamer{
		subscriptions:    subscriptions,
		channelIDToName:  channelIDToName,
		channelIDMapping: make(map[string]string, channelLen),
		addedProgrammes:  make(map[string]bool, approxProgrammeLen),
		addedChannels:    make(map[string][]string, channelLen),
	}
}

func (s *Streamer) WriteToGzip(ctx context.Context, w io.Writer) (int64, error) {
	gzWriter, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
	defer func() { _ = gzWriter.Close() }()
	return s.WriteTo(ctx, gzWriter)
}

func (s *Streamer) WriteTo(ctx context.Context, w io.Writer) (int64, error) {
	if len(s.subscriptions) == 0 {
		return 0, fmt.Errorf("no EPG sources found")
	}

	bytesCounter := ioutil.NewCountWriter(w)
	encoder := xmltv.NewEncoder(bytesCounter)
	defer func() { _ = encoder.Close() }()

	var decoders []*decoderWrapper
	for _, sub := range s.subscriptions {
		for _, url := range sub.EPGs() {
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
		if err := decoder.StartBuffering(ctx); err != nil {
			return bytesCounter.Count(), err
		}
	}

	for _, decoder := range decoders {
		if err := s.processChannels(ctx, decoder, encoder); err != nil {
			return bytesCounter.Count(), err
		}
	}

	for _, decoder := range decoders {
		if err := s.processProgrammes(ctx, decoder, encoder); err != nil {
			return bytesCounter.Count(), err
		}
	}

	count := bytesCounter.Count()
	if count == 0 {
		return count, fmt.Errorf("no data in subscriptions")
	}

	return count, encoder.WriteFooter()
}

func (s *Streamer) processChannels(ctx context.Context, decoder *decoderWrapper, encoder Encoder) error {
	decoder.StopBuffer()
	defer func() { _ = decoder.StartBuffering(ctx) }()

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
			if item == nil {
				return nil
			}

			if _, ok := item.(xmltv.Programme); ok {
				decoder.AddToBuffer(item)
				return nil
			}

			if channel, ok := item.(xmltv.Channel); ok {
				channel.Icons = s.processIcons(decoder.subscription, channel.Icons)
				if s.processChannel(&channel, decoder.sourceURL) {
					if err := encoder.Encode(channel); err != nil {
						return err
					}
				}
			}
		}
	}
}

func (s *Streamer) processProgrammes(ctx context.Context, decoder *decoderWrapper, encoder Encoder) error {
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
			if item == nil {
				return nil
			}

			if programme, ok := item.(xmltv.Programme); ok {
				programme.Icons = s.processIcons(decoder.subscription, programme.Icons)
				if s.processProgramme(&programme, decoder.sourceURL) {
					if err := encoder.Encode(programme); err != nil {
						return err
					}
				}
			}
		}
	}
}

func (s *Streamer) processChannel(channel *xmltv.Channel, sourceURL string) (allowed bool) {
	originalID := channel.ID
	compositeKey := listing.GenerateHashID(originalID, sourceURL)

	candidateIDs := make([]string, 0, 1+len(channel.DisplayNames))
	candidateIDs = append(candidateIDs, originalID)

	currentChannelNames := make([]string, 0, len(channel.DisplayNames))
	for _, displayName := range channel.DisplayNames {
		currentChannelNames = append(currentChannelNames, displayName.Value)
		tvgID := listing.GenerateHashID(displayName.Value)
		candidateIDs = append(candidateIDs, tvgID)
	}

	for _, id := range candidateIDs {
		if channelName, exists := s.channelIDToName[id]; exists {
			if existingNames, ok := s.addedChannels[id]; ok {
				if !s.channelNamesMatch(currentChannelNames, existingNames) {
					return false
				}
				s.channelIDMapping[compositeKey] = id
				return false
			}

			s.channelIDMapping[compositeKey] = id
			s.addedChannels[id] = currentChannelNames
			channel.ID = id

			if channelName != "" {
				channel.DisplayNames = []xmltv.CommonElement{
					{Value: channelName},
				}
			}

			return true
		}
	}

	return false
}

func (s *Streamer) channelNamesMatch(currentNames, existingNames []string) bool {
	for _, currentName := range currentNames {
		for _, existingName := range existingNames {
			if currentName == existingName {
				return true
			}
		}
	}
	return false
}

func (s *Streamer) processProgramme(programme *xmltv.Programme, sourceURL string) (allowed bool) {
	compositeKey := listing.GenerateHashID(programme.Channel, sourceURL)

	mappedChannel, exists := s.channelIDMapping[compositeKey]
	if !exists {
		return false
	}

	programme.Channel = mappedChannel

	key := programme.Channel
	if programme.Start != nil {
		key += programme.Start.Time.String()
	}
	if programme.ID != "" {
		key += programme.ID
	}

	if s.addedProgrammes[key] {
		return false
	}

	s.addedProgrammes[key] = true
	return true
}

func (s *Streamer) processIcons(sub listing.EPG, icons []xmltv.Icon) []xmltv.Icon {
	if len(icons) == 0 {
		return icons
	}

	gen := sub.URLGenerator()
	if gen == nil {
		return icons
	}

	needsUpdate := false
	for _, icon := range icons {
		if icon.Source != "" {
			needsUpdate = true
			break
		}
	}

	if !needsUpdate {
		return icons
	}

	for i := range icons {
		if icons[i].Source == "" {
			continue
		}

		link, err := gen.CreateFileURL(
			urlgen.ProviderInfo{
				ProviderType: urlgen.ProviderTypeEPG,
				ProviderName: sub.Name(),
			}, icons[i].Source)
		if err != nil {
			continue
		}
		icons[i].Source = link.String()
	}

	return icons
}
