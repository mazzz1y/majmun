package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
)

type EPG struct {
	Name    string             `yaml:"name"`
	Sources common.StringOrArr `yaml:"sources"`
	Proxy   proxy.Proxy        `yaml:"proxy,omitempty"`
}

func (e *EPG) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("EPG name is required")
	}
	if len(e.Sources) == 0 {
		return fmt.Errorf("EPG sources are required")
	}
	for i, source := range e.Sources {
		if source == "" {
			return fmt.Errorf("EPG source[%d] cannot be empty", i)
		}
	}

	err := e.Proxy.ValidateOverride()
	if err != nil {
		return fmt.Errorf("epg proxy validation error: %v", err)
	}

	return nil
}
