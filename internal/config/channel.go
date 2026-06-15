package config

import (
	"fmt"
	"majmun/internal/config/common"
)

type Channel struct {
	Name    string             `yaml:"name"`
	Fields  []ChannelField     `yaml:"fields,omitempty"`
	Sources common.StringOrArr `yaml:"sources"`
	Playout Playout            `yaml:"playout,omitempty"`
}

type ChannelField struct {
	Selector *common.Selector `yaml:"selector"`
	Template *common.Template `yaml:"template"`
}

func (f *ChannelField) Validate() error {
	if f.Selector == nil {
		return fmt.Errorf("selector is required")
	}
	if err := f.Selector.Validate(); err != nil {
		return err
	}
	if f.Selector.Type != common.SelectorAttr && f.Selector.Type != common.SelectorTag {
		return fmt.Errorf("selector must be attr/<key> or tag/<key>, got '%s'", f.Selector.Raw)
	}
	if f.Template == nil {
		return fmt.Errorf("template is required")
	}
	return nil
}

func (c *Channel) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("sources is required")
	}
	for i, source := range c.Sources {
		if source == "" {
			return fmt.Errorf("sources[%d] cannot be empty", i)
		}
	}
	for i, field := range c.Fields {
		if err := field.Validate(); err != nil {
			return fmt.Errorf("fields[%d]: %w", i, err)
		}
	}
	if err := c.Playout.Validate(); err != nil {
		return fmt.Errorf("playout: %w", err)
	}
	return nil
}
