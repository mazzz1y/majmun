package proxy

import (
	"fmt"
	"majmun/internal/config/common"
	"slices"
)

var reservedSegmenterVars = []string{
	"segment_duration",
	"max_segments",
	"segment_path",
	"playlist_path",
}

type Segmenter struct {
	Command      common.StringOrArr `yaml:"command,omitempty"`
	TemplateVars []common.NameValue `yaml:"template_variables,omitempty"`
	EnvVars      []common.NameValue `yaml:"env_variables,omitempty"`

	SegmentDuration *int             `yaml:"segment_duration,omitempty"`
	MaxSegments     *int             `yaml:"max_segments,omitempty"`
	InitSegments    *int             `yaml:"init_segments,omitempty"`
	ReadyTimeout    *common.Duration `yaml:"ready_timeout,omitempty"`
}

func (s *Segmenter) Validate() error {
	for i, templateVar := range s.TemplateVars {
		if err := templateVar.Validate(); err != nil {
			return fmt.Errorf("template_variables[%d]: %w", i, err)
		}
		if slices.Contains(reservedSegmenterVars, templateVar.Name) {
			return fmt.Errorf("template_variables[%d]: %q is a reserved variable", i, templateVar.Name)
		}
	}
	for i, envVar := range s.EnvVars {
		if err := envVar.Validate(); err != nil {
			return fmt.Errorf("env_variables[%d]: %w", i, err)
		}
	}
	if s.SegmentDuration != nil && *s.SegmentDuration < 1 {
		return fmt.Errorf("segment_duration must be at least 1")
	}
	if s.MaxSegments != nil && *s.MaxSegments < 1 {
		return fmt.Errorf("max_segments must be at least 1")
	}
	if s.InitSegments != nil && *s.InitSegments < 1 {
		return fmt.Errorf("init_segments must be at least 1")
	}
	if s.MaxSegments != nil && s.InitSegments != nil && *s.InitSegments > *s.MaxSegments {
		return fmt.Errorf("init_segments cannot exceed max_segments")
	}
	return nil
}
