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

func (c *HTTPClient) Validate() error {
	return c.ValidateGlobal()
}

func (c *HTTPClient) ValidateGlobal() error {
	if err := c.validateGlobalCache(); err != nil {
		return err
	}
	for i, header := range c.HTTPHeaders {
		if err := header.Validate(); err != nil {
			return fmt.Errorf("http_client: header[%d]: %w", i, err)
		}
	}
	return nil
}

func (c *HTTPClient) ValidateProxyOverride() error {
	if err := c.Cache.ValidateProxyOverride(); err != nil {
		return err
	}
	for i, header := range c.HTTPHeaders {
		if err := header.Validate(); err != nil {
			return fmt.Errorf("http_client: header[%d]: %w", i, err)
		}
	}
	return nil
}

func (c *HTTPClient) validateGlobalCache() error {
	cache := &c.Cache
	enabled := cache.Enabled != nil && *cache.Enabled
	disabled := cache.Enabled != nil && !*cache.Enabled

	if disabled {
		return nil
	}

	if enabled {
		if cache.Path == nil || *cache.Path == "" {
			return fmt.Errorf("http_client.cache: path is required when cache is enabled")
		}
		if cache.TTL == nil || *cache.TTL <= 0 {
			return fmt.Errorf("http_client.cache: ttl must be positive when cache is enabled")
		}
		if cache.Retention == nil || *cache.Retention <= 0 {
			return fmt.Errorf("http_client.cache: retention must be positive when cache is enabled")
		}
	}

	return nil
}

func (c *Cache) ValidateProxyOverride() error {
	if c.Path != nil {
		return fmt.Errorf("http_client.cache.path: path can only be configured at the global level")
	}
	if c.Enabled != nil && !*c.Enabled {
		if c.TTL != nil {
			return fmt.Errorf("http_client.cache: ttl cannot be set when cache is disabled")
		}
	}
	if c.TTL != nil && *c.TTL <= 0 {
		return fmt.Errorf("http_client.cache: ttl must be positive")
	}
	if c.Retention != nil && *c.Retention < 0 {
		return fmt.Errorf("http_client.cache: retention cannot be negative")
	}
	return nil
}
