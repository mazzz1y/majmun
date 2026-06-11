package xmltv

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"majmun/internal/hashid"
	"majmun/internal/ioutil"
	"majmun/internal/listing"
	"majmun/internal/logging"
	"majmun/internal/parser/xmltv"
	"majmun/internal/urlgen"
	"slices"
	"strconv"
	"time"
)

type Streamer struct {
	subscriptions    []listing.EPG
	channels         []listing.Channel
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

func NewStreamer(subs []listing.EPG, channels []listing.Channel, channelIDToName map[string]string) *Streamer {
	subscriptions := subs
	channelLen := len(channelIDToName)
	approxProgrammeLen := 300 * channelLen

	return &Streamer{
		subscriptions:    subscriptions,
		channels:         channels,
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
	if len(s.subscriptions) == 0 && len(s.channels) == 0 {
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

	failed := make([]bool, len(decoders))
	for i, decoder := range decoders {
		if err := decoder.StartBuffering(ctx); err != nil {
			if decoder.subscription.SkipOnError() {
				logging.Error(ctx, err, "EPG source failed, skipping",
					"provider", decoder.subscription.Name(),
					"source", decoder.sourceURL)
				failed[i] = true
				continue
			}
			return bytesCounter.Count(), err
		}
	}

	for i, decoder := range decoders {
		if failed[i] {
			continue
		}
		if err := s.processChannels(ctx, decoder, encoder); err != nil {
			if decoder.subscription.SkipOnError() {
				logging.Error(ctx, err, "EPG source failed mid-stream, skipping",
					"provider", decoder.subscription.Name(),
					"source", decoder.sourceURL)
				failed[i] = true
				continue
			}
			return bytesCounter.Count(), err
		}
	}

	if err := s.writeGeneratedChannels(encoder); err != nil {
		return bytesCounter.Count(), err
	}

	for i, decoder := range decoders {
		if failed[i] {
			continue
		}
		if err := s.processProgrammes(ctx, decoder, encoder); err != nil {
			if decoder.subscription.SkipOnError() {
				logging.Error(ctx, err, "EPG source failed mid-stream, skipping",
					"provider", decoder.subscription.Name(),
					"source", decoder.sourceURL)
				failed[i] = true
				continue
			}
			return bytesCounter.Count(), err
		}
	}

	if err := s.writeGeneratedProgrammes(ctx, encoder); err != nil {
		return bytesCounter.Count(), err
	}

	count := bytesCounter.Count()
	if count == 0 {
		return count, fmt.Errorf("no data in subscriptions")
	}

	return count, encoder.WriteFooter()
}

func (s *Streamer) writeGeneratedChannels(encoder Encoder) error {
	for _, ch := range s.channels {
		channel := xmltv.Channel{
			ID:           ch.ID(),
			DisplayNames: []xmltv.CommonElement{{Value: ch.Name()}},
		}
		logo, err := listing.ChannelLogoURL(ch)
		if err != nil {
			return err
		}
		if logo != "" {
			channel.Icons = []xmltv.Icon{{Source: logo}}
		}
		if err := encoder.Encode(channel); err != nil {
			return err
		}
	}
	return nil
}

func (s *Streamer) writeGeneratedProgrammes(ctx context.Context, encoder Encoder) error {
	now := time.Now()
	for _, ch := range s.channels {
		programmes, err := ch.Programmes(ctx, now)
		if err != nil {
			logging.Error(ctx, err, "failed to build channel EPG", "channel_name", ch.Name())
			continue
		}
		for _, p := range programmes {
			programme := xmltv.Programme{
				Channel: ch.ID(),
				Titles:  []xmltv.CommonElement{{Value: p.Title}},
				Start:   &xmltv.Time{Time: p.Start},
				Stop:    &xmltv.Time{Time: p.Stop},
			}
			if p.Description != "" {
				programme.Descriptions = []xmltv.CommonElement{{Value: p.Description}}
			}
			if p.Category != "" {
				programme.Categories = []xmltv.CommonElement{{Value: p.Category}}
			}
			if d, ok := parseProgrammeDate(p.Date); ok {
				programme.Date = &d
			}
			programme.EpisodeNums = episodeNums(p.Season, p.Episode)
			if err := encoder.Encode(programme); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseProgrammeDate(date string) (xmltv.Date, bool) {
	if date == "" {
		return xmltv.Date{}, false
	}
	t, err := time.Parse("20060102", date)
	if err != nil {
		return xmltv.Date{}, false
	}
	return xmltv.Date(t), true
}

func episodeNums(season, episode int) []xmltv.EpisodeNum {
	if episode <= 0 {
		return nil
	}
	nums := make([]xmltv.EpisodeNum, 0, 2)
	if season > 0 {
		nums = append(nums,
			xmltv.EpisodeNum{System: "xmltv_ns", Value: fmt.Sprintf("%d.%d.", season-1, episode-1)},
			xmltv.EpisodeNum{System: "onscreen", Value: fmt.Sprintf("S%02dE%02d", season, episode)},
		)
	} else {
		nums = append(nums,
			xmltv.EpisodeNum{System: "xmltv_ns", Value: fmt.Sprintf(".%d.", episode-1)},
			xmltv.EpisodeNum{System: "onscreen", Value: fmt.Sprintf("E%02d", episode)},
		)
	}
	return nums
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
	compositeKey := hashid.New(originalID, sourceURL)

	candidateIDs := make([]string, 0, 1+len(channel.DisplayNames))
	candidateIDs = append(candidateIDs, originalID)

	currentChannelNames := make([]string, 0, len(channel.DisplayNames))
	for _, displayName := range channel.DisplayNames {
		currentChannelNames = append(currentChannelNames, displayName.Value)
		tvgID := hashid.New(displayName.Value)
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
		if slices.Contains(existingNames, currentName) {
			return true
		}
	}
	return false
}

func (s *Streamer) processProgramme(programme *xmltv.Programme, sourceURL string) (allowed bool) {
	compositeKey := hashid.New(programme.Channel, sourceURL)

	mappedChannel, exists := s.channelIDMapping[compositeKey]
	if !exists {
		return false
	}

	programme.Channel = mappedChannel

	key := programme.Channel
	if programme.Start != nil {
		key += strconv.FormatInt(programme.Start.Time.Unix(), 36)
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
				ProviderID:   sub.ID(),
			}, icons[i].Source)
		if err != nil {
			continue
		}
		icons[i].Source = link.String()
	}

	return icons
}
