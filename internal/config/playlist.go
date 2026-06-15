package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
)

type Playlist struct {
	Name        string             `yaml:"name"`
	Sources     common.StringOrArr `yaml:"sources,omitempty"`
	Channels    []Channel          `yaml:"channels,omitempty"`
	Proxy       proxy.Proxy        `yaml:"proxy,omitempty"`
	Playout     Playout            `yaml:"playout,omitempty"`
	SkipOnError bool               `yaml:"skip_on_error,omitempty"`
}

func (p *Playlist) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(p.Sources) == 0 && len(p.Channels) == 0 {
		return fmt.Errorf("at least one of sources or channels is required")
	}
	for i, source := range p.Sources {
		if source == "" {
			return fmt.Errorf("sources[%d] cannot be empty", i)
		}
	}
	for i, ch := range p.Channels {
		if err := ch.Validate(); err != nil {
			return fmt.Errorf("channels[%d] validation failed: %w", i, err)
		}
	}
	if err := p.Proxy.ValidateOverride(); err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	if err := p.Playout.Validate(); err != nil {
		return fmt.Errorf("playout: %w", err)
	}
	return nil
}
