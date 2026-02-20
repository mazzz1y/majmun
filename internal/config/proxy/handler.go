package proxy

import (
	"fmt"
	"majmun/internal/config/common"
)

type Handler struct {
	Command      common.StringOrArr `yaml:"command,omitempty"`
	TemplateVars []common.NameValue `yaml:"template_variables,omitempty"`
	EnvVars      []common.NameValue `yaml:"env_variables,omitempty"`
}

func mergeHandlerVars(base, override *Handler) {
	override.TemplateVars = common.MergeNameValues(base.TemplateVars, override.TemplateVars)
	override.EnvVars = common.MergeNameValues(base.EnvVars, override.EnvVars)
}

func (h *Handler) Validate() error {
	for i, templateVar := range h.TemplateVars {
		if err := templateVar.Validate(); err != nil {
			return fmt.Errorf("template_variables[%d]: %w", i, err)
		}
	}

	for i, envVar := range h.EnvVars {
		if err := envVar.Validate(); err != nil {
			return fmt.Errorf("env_variables[%d]: %w", i, err)
		}
	}

	return nil
}
