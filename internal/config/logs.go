package config

import (
	"fmt"
	"slices"
)

var (
	validLogLevels  = []string{"debug", "info", "warn", "error"}
	validLogFormats = []string{"text", "json"}
)

type Logs struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func (l *Logs) Validate() error {
	if l.Level != "" && !slices.Contains(validLogLevels, l.Level) {
		return fmt.Errorf("invalid log level '%s', must be one of: %v", l.Level, validLogLevels)
	}
	if l.Format != "" && !slices.Contains(validLogFormats, l.Format) {
		return fmt.Errorf("invalid log format '%s', must be one of: %v", l.Format, validLogFormats)
	}
	return nil
}
