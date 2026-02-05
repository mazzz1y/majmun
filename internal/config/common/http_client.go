package common

import (
	"fmt"
)

type HTTPClient struct {
	Cache   Cache       `yaml:"cache"`
	Headers []NameValue `yaml:"headers"`
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
		return fmt.Errorf("cache.path can only be configured at the global level")
	}
	if c.Cache.Enabled != nil && !*c.Cache.Enabled && c.Cache.TTL != nil {
		return fmt.Errorf("cache.ttl cannot be set when cache is disabled")
	}
	if c.Cache.TTL != nil && *c.Cache.TTL <= 0 {
		return fmt.Errorf("cache.ttl must be positive")
	}
	if c.Cache.Retention != nil && *c.Cache.Retention < 0 {
		return fmt.Errorf("cache.retention cannot be negative")
	}
	for i, header := range c.Headers {
		if err := header.Validate(); err != nil {
			return fmt.Errorf("headers[%d]: %w", i, err)
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
			return fmt.Errorf("cache.path is required when cache is enabled")
		}
		if cache.TTL == nil || *cache.TTL <= 0 {
			return fmt.Errorf("cache.ttl must be positive when cache is enabled")
		}
		if cache.Retention == nil || *cache.Retention <= 0 {
			return fmt.Errorf("cache.retention must be positive when cache is enabled")
		}
	}
	for i, header := range c.Headers {
		if err := header.Validate(); err != nil {
			return fmt.Errorf("headers[%d]: %w", i, err)
		}
	}
	return nil
}
