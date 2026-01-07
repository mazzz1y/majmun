package common

import (
	"fmt"
)

type HTTPClient struct {
	Cache       Cache       `yaml:"cache"`
	HTTPHeaders []NameValue `yaml:"headers"`
}

type Cache struct {
	Enabled     *bool     `yaml:"enabled"`
	Path        *string   `yaml:"path"`
	TTL         *Duration `yaml:"ttl"`
	Retention   *Duration `yaml:"retention"`
	Compression *bool     `yaml:"compression"`
}

func (c *HTTPClient) ValidateProxyOverride() error {
	if c.Cache.Path != nil {
		return fmt.Errorf("path can only be configured at the global level")
	}
	if c.Cache.Enabled != nil && !*c.Cache.Enabled {
		if c.Cache.TTL != nil {
			return fmt.Errorf("ttl cannot be set when cache is disabled")
		}
	}
	if c.Cache.TTL != nil && *c.Cache.TTL <= 0 {
		return fmt.Errorf("ttl must be positive")
	}
	if c.Cache.Retention != nil && *c.Cache.Retention < 0 {
		return fmt.Errorf("retention cannot be negative")
	}

	for i, header := range c.HTTPHeaders {
		if err := header.Validate(); err != nil {
			return fmt.Errorf("header[%d]: %w", i, err)
		}
	}
	return nil
}

func (c *HTTPClient) ValidateProxyGlobal() error {
	cache := &c.Cache
	enabled := cache.Enabled != nil && *cache.Enabled
	disabled := cache.Enabled != nil && !*cache.Enabled

	if disabled {
		return nil
	}

	if enabled {
		if cache.Path == nil || *cache.Path == "" {
			return fmt.Errorf("path is required when cache is enabled")
		}
		if cache.TTL == nil || *cache.TTL <= 0 {
			return fmt.Errorf("ttl must be positive when cache is enabled")
		}
		if cache.Retention == nil || *cache.Retention <= 0 {
			return fmt.Errorf("retention must be positive when cache is enabled")
		}
	}
	for i, header := range c.HTTPHeaders {
		if err := header.Validate(); err != nil {
			return fmt.Errorf("header[%d]: %w", i, err)
		}
	}
	return nil
}
