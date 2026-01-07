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
		return fmt.Errorf("client name is required")
	}
	if c.Secret == "" {
		return fmt.Errorf("client secret is required")
	}

	for _, p := range c.Playlists {
		if !playlistNames[p] {
			return fmt.Errorf("client references unknown playlist: %s", p)
		}
	}

	for _, epg := range c.EPGs {
		if !epgNames[epg] {
			return fmt.Errorf("client references unknown EPG: %s", epg)
		}
	}

	err := c.Proxy.ValidateOverride()
	if err != nil {
		return fmt.Errorf("client proxy validation error: %v", err)
	}

	return nil
}
