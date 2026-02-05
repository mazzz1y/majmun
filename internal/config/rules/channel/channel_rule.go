package channel

import (
	"fmt"
	"majmun/internal/config/common"

	"gopkg.in/yaml.v3"
)

type Rules []*Rule

func (c *Rules) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected a sequence for channel_rules, got %s", value.Tag)
	}
	rules := make(Rules, len(value.Content))
	for i, node := range value.Content {
		rule := &Rule{}
		if err := node.Decode(rule); err != nil {
			return fmt.Errorf("channel_rules[%d]: %w", i, err)
		}
		rules[i] = rule
	}
	*c = rules
	return nil
}

type Rule struct {
	Validate func() error

	SetField      *SetFieldRule      `yaml:"set_field,omitempty"`
	RemoveField   *RemoveFieldRule   `yaml:"remove_field,omitempty"`
	RemoveChannel *RemoveChannelRule `yaml:"remove_channel,omitempty"`
	MarkHidden    *MarkHiddenRule    `yaml:"mark_hidden,omitempty"`
}

func (r *Rule) UnmarshalYAML(value *yaml.Node) error {
	type rawRule Rule
	var rr rawRule

	if err := common.DecodeStrict(value, &rr); err != nil {
		return err
	}

	rule := Rule(rr)

	switch {
	case rule.SetField != nil:
		rule.Validate = rule.SetField.Validate
	case rule.RemoveField != nil:
		rule.Validate = rule.RemoveField.Validate
	case rule.RemoveChannel != nil:
		rule.Validate = rule.RemoveChannel.Validate
	case rule.MarkHidden != nil:
		rule.Validate = rule.MarkHidden.Validate
	default:
		return fmt.Errorf("unrecognized rule type")
	}

	*r = rule
	return nil
}
