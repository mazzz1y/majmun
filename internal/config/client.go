package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
)

type Client struct {
	Name      string             `yaml:"name"`
	Secret    string             `yaml:"secret"`
	Playlists common.StringOrArr `yaml:"playlists"`
	EPGs      common.StringOrArr `yaml:"epgs"`
	Proxy     proxy.Proxy        `yaml:"proxy,omitempty"`
}

func (c *Client) Validate(playlistNames, epgNames map[string]bool) error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Secret == "" {
		return fmt.Errorf("secret is required")
	}
	for _, p := range c.Playlists {
		if !playlistNames[p] {
			return fmt.Errorf("unknown playlist: %s", p)
		}
	}
	for _, epg := range c.EPGs {
		if !epgNames[epg] {
			return fmt.Errorf("unknown epg: %s", epg)
		}
	}

	if err := c.Proxy.ValidateOverride(); err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	return nil
}
