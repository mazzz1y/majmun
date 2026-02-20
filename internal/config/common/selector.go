package common

import (
	"fmt"
	"strings"
)

type SelectorType string

const (
	SelectorName SelectorType = "name"
	SelectorAttr SelectorType = "attr"
	SelectorTag  SelectorType = "tag"
	SelectorURL  SelectorType = "url"
)

type Selector struct {
	Type  SelectorType `yaml:"-"`
	Value string       `yaml:"-"`
	Raw   string       `yaml:",inline"`
}

func (s *Selector) UnmarshalYAML(unmarshal func(any) error) error {
	var raw string
	if err := unmarshal(&raw); err != nil {
		return err
	}

	s.Raw = raw

	if raw == "name" {
		s.Type = SelectorName
		s.Value = ""
		return nil
	}

	if raw == "url" {
		s.Type = SelectorURL
		s.Value = ""
		return nil
	}

	if strings.HasPrefix(raw, "attr/") {
		s.Type = SelectorAttr
		s.Value = strings.TrimPrefix(raw, "attr/")
		if s.Value == "" {
			return fmt.Errorf("selector: attr selector requires a value (e.g., attr/tvg-id)")
		}
		return nil
	}

	if strings.HasPrefix(raw, "tag/") {
		s.Type = SelectorTag
		s.Value = strings.TrimPrefix(raw, "tag/")
		if s.Value == "" {
			return fmt.Errorf("selector: tag selector requires a value (e.g., tag/mytag)")
		}
		return nil
	}

	return fmt.Errorf("selector: invalid format '%s', expected 'name', 'url', 'attr/<value>', or 'tag/<value>'", raw)
}

func (s *Selector) Validate() error {
	switch s.Type {
	case "", SelectorName, SelectorURL:
		return nil
	case SelectorAttr, SelectorTag:
		if s.Value == "" {
			return fmt.Errorf("%s requires a value", s.Type)
		}
		return nil
	default:
		return fmt.Errorf("unknown type: %s", s.Type)
	}
}
