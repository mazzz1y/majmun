package playlist

import (
	"fmt"
	"majmun/internal/config/common"

	"gopkg.in/yaml.v3"
)

type Rules []*Rule

func (p *Rules) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected a sequence for playlist_rules, got %s", value.Tag)
	}
	rules := make(Rules, len(value.Content))
	for i, node := range value.Content {
		rule := &Rule{}
		if err := node.Decode(rule); err != nil {
			return fmt.Errorf("playlist_rules[%d]: %w", i, err)
		}
		rules[i] = rule
	}
	*p = rules
	return nil
}

type Rule struct {
	Validate func() error

	RemoveDuplicates *RemoveDuplicatesRule `yaml:"remove_duplicates,omitempty"`
	MergeChannels    *MergeDuplicatesRule  `yaml:"merge_duplicates,omitempty"`
	SortRule         *Sort                 `yaml:"sort,omitempty"`
}

func (r *Rule) UnmarshalYAML(value *yaml.Node) error {
	type rawRule Rule
	var rr rawRule

	if err := common.DecodeStrict(value, &rr); err != nil {
		return err
	}

	rule := Rule(rr)

	switch {
	case rule.RemoveDuplicates != nil:
		rule.Validate = rule.RemoveDuplicates.Validate
	case rule.MergeChannels != nil:
		rule.Validate = rule.MergeChannels.Validate
	case rule.SortRule != nil:
		rule.Validate = rule.SortRule.Validate
	default:
		return fmt.Errorf("unrecognized rule type")
	}

	*r = rule
	return nil
}
