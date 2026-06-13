package proxy

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/shell"
)

// RunnerConfig is the shared shape the stream pool needs to launch an FFmpeg process,
// satisfied by both Segmenter (upstream proxied streams) and Playout (generated channels).
type RunnerConfig interface {
	GetCommand() common.StringOrArr
	GetEnvVars() []common.NameValue
	GetTemplateVars() []common.NameValue
	GetInitSegments() *int
	GetReadyTimeout() *common.Duration
}

type Segmenter struct {
	Command      common.StringOrArr `yaml:"command,omitempty"`
	TemplateVars []common.NameValue `yaml:"template_variables,omitempty"`
	EnvVars      []common.NameValue `yaml:"env_variables,omitempty"`

	InitSegments *int             `yaml:"init_segments,omitempty"`
	ReadyTimeout *common.Duration `yaml:"ready_timeout,omitempty"`
}

func (s *Segmenter) GetCommand() common.StringOrArr      { return s.Command }
func (s *Segmenter) GetEnvVars() []common.NameValue      { return s.EnvVars }
func (s *Segmenter) GetTemplateVars() []common.NameValue { return s.TemplateVars }
func (s *Segmenter) GetInitSegments() *int               { return s.InitSegments }
func (s *Segmenter) GetReadyTimeout() *common.Duration   { return s.ReadyTimeout }

func mergeSegmenterVars(base, override *Segmenter) {
	override.TemplateVars = common.MergeNameValues(base.TemplateVars, override.TemplateVars)
	override.EnvVars = common.MergeNameValues(base.EnvVars, override.EnvVars)
}

func (s *Segmenter) Validate() error {
	for i, templateVar := range s.TemplateVars {
		if err := templateVar.Validate(); err != nil {
			return fmt.Errorf("template_variables[%d]: %w", i, err)
		}
		if shell.IsReservedVar(templateVar.Name) {
			return fmt.Errorf("template_variables[%d]: %q is a reserved variable", i, templateVar.Name)
		}
	}
	for i, envVar := range s.EnvVars {
		if err := envVar.Validate(); err != nil {
			return fmt.Errorf("env_variables[%d]: %w", i, err)
		}
	}
	if s.InitSegments != nil && *s.InitSegments < 1 {
		return fmt.Errorf("init_segments must be at least 1")
	}
	return nil
}
