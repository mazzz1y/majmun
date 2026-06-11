package store

import "majmun/internal/hashid"

type Store struct {
	channels []*Channel
	counter  int
}

func NewStore() *Store {
	return &Store{
		channels: make([]*Channel, 0),
		counter:  0,
	}
}

func (s *Store) Add(channel *Channel) {
	ensureTvgID(channel)
	s.channels = append(s.channels, channel)
	s.counter++
}

func (s *Store) Replace(channels []*Channel) {
	s.channels = channels
}

func (s *Store) All() []*Channel {
	return s.channels
}

func (s *Store) Len() int {
	return s.counter
}

func ensureTvgID(ch *Channel) {
	if tvgID, exists := ch.GetAttr("tvg-id"); exists && tvgID != "" {
		return
	}
	if tvgName, exists := ch.GetAttr("tvg-name"); exists && tvgName != "" {
		ch.SetAttr("tvg-id", hashid.New(tvgName))
	} else {
		ch.SetAttr("tvg-id", hashid.New(ch.Name()))
	}
}
