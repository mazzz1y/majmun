package proxy

import "fmt"

type Error struct {
	Handler           `yaml:",inline"`
	UpstreamError     Handler `yaml:"upstream_error"`
	RateLimitExceeded Handler `yaml:"rate_limit_exceeded"`
	LinkExpired       Handler `yaml:"link_expired"`
}

func (e *Error) Validate() error {
	if err := e.Handler.Validate(); err != nil {
		return err
	}
	if err := e.UpstreamError.Validate(); err != nil {
		return fmt.Errorf("upstream_error: %w", err)
	}
	if err := e.RateLimitExceeded.Validate(); err != nil {
		return fmt.Errorf("rate_limit_exceeded: %w", err)
	}
	if err := e.LinkExpired.Validate(); err != nil {
		return fmt.Errorf("link_expired: %w", err)
	}
	return nil
}
