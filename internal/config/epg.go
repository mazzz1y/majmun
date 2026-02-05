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
		return fmt.Errorf("name is required")
	}
	if len(e.Sources) == 0 {
		return fmt.Errorf("sources is required")
	}
	for i, source := range e.Sources {
		if source == "" {
			return fmt.Errorf("sources[%d] cannot be empty", i)
		}
	}
	if err := e.Proxy.ValidateOverride(); err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	return nil
}
