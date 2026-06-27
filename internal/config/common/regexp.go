package common

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

type RegexpArr []*regexp.Regexp

func (r *RegexpArr) UnmarshalYAML(value *yaml.Node) error {
	patterns, err := unmarshalStringOrArray(value)
	if err != nil {
		return err
	}
	if len(patterns) == 0 {
		return nil
	}

	regexps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
		}
		regexps = append(regexps, compiled)
	}

	*r = regexps
	return nil
}

func (r *RegexpArr) ToArray() []*regexp.Regexp {
	return *r
}

func MustRegexpArr(patterns ...string) RegexpArr {
	arr := make(RegexpArr, len(patterns))
	for i, p := range patterns {
		arr[i] = regexp.MustCompile(p)
	}
	return arr
}
