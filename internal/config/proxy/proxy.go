package proxy

import (
	"fmt"
	"majmun/internal/config/common"

	"gopkg.in/yaml.v3"
)

type Proxy struct {
	Enabled           *bool             `yaml:"enabled"`
	ConcurrentStreams int64             `yaml:"concurrency"`
	HTTPClient        common.HTTPClient `yaml:"http_client,omitempty"`
	Stream            Handler           `yaml:"stream,omitempty"`
	Segmenter         Segmenter         `yaml:"segmenter,omitempty"`
	Error             Error             `yaml:"error,omitempty"`
}

func (p *Proxy) ValidateGlobal() error {
	if p.ConcurrentStreams < 0 {
		return fmt.Errorf("concurrency cannot be negative")
	}
	if err := p.Stream.Validate(); err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	if err := p.Segmenter.Validate(); err != nil {
		return fmt.Errorf("segmenter: %w", err)
	}
	if err := p.HTTPClient.ValidateProxyGlobal(); err != nil {
		return fmt.Errorf("http_client: %w", err)
	}
	if err := p.Error.Validate(); err != nil {
		return fmt.Errorf("error: %w", err)
	}
	return nil
}

func (p *Proxy) ValidateOverride() error {
	if p.ConcurrentStreams < 0 {
		return fmt.Errorf("concurrency cannot be negative")
	}
	if err := p.Stream.Validate(); err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	if err := p.Segmenter.Validate(); err != nil {
		return fmt.Errorf("segmenter: %w", err)
	}
	if err := p.HTTPClient.ValidateProxyOverride(); err != nil {
		return fmt.Errorf("http_client: %w", err)
	}
	if err := p.Error.Validate(); err != nil {
		return fmt.Errorf("error: %w", err)
	}
	return nil
}

func (p *Proxy) UnmarshalYAML(value *yaml.Node) error {
	var enabled bool
	if err := value.Decode(&enabled); err == nil {
		p.Enabled = &enabled
		return nil
	}

	type proxyYAML Proxy
	return common.DecodeStrict(value, (*proxyYAML)(p))
}
