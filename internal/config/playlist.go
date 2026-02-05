package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
)

type Playlist struct {
	Name    string             `yaml:"name"`
	Sources common.StringOrArr `yaml:"sources"`
	Proxy   proxy.Proxy        `yaml:"proxy,omitempty"`
}

func (p *Playlist) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(p.Sources) == 0 {
		return fmt.Errorf("sources is required")
	}
	for i, source := range p.Sources {
		if source == "" {
			return fmt.Errorf("sources[%d] cannot be empty", i)
		}
	}

	if err := p.Proxy.ValidateOverride(); err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	return nil
}
