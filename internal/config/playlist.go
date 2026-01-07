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
		return fmt.Errorf("playlist name is required")
	}
	if len(p.Sources) == 0 {
		return fmt.Errorf("playlist sources are required")
	}
	for i, source := range p.Sources {
		if source == "" {
			return fmt.Errorf("playlist source[%d] cannot be empty", i)
		}
	}

	err := p.Proxy.ValidateOverride()
	if err != nil {
		return fmt.Errorf("playlist proxy validation error: %v", err)
	}

	return nil
}
