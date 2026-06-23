package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/shell"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var validPlayoutOrders = map[string]bool{"sequential": true, "shuffle": true, "interleave": true}

type Playout struct {
	Command      common.StringOrArr `yaml:"command,omitempty"`
	TemplateVars []common.NameValue `yaml:"template_variables,omitempty"`
	EnvVars      []common.NameValue `yaml:"env_variables,omitempty"`
	InitSegments int                `yaml:"init_segments,omitempty"`
	ReadyTimeout common.Duration    `yaml:"ready_timeout,omitempty"`

	StateDir        string             `yaml:"state_dir,omitempty"`
	Logo            string             `yaml:"logo,omitempty"`
	Extensions      common.StringOrArr `yaml:"extensions,omitempty"`
	Order           string             `yaml:"order,omitempty"`
	RefreshInterval *common.Duration   `yaml:"refresh_interval,omitempty"`
	EPGDuration     common.Duration    `yaml:"epg_duration,omitempty"`
	ScheduleSwapAt  string             `yaml:"schedule_swap_at,omitempty"`
}

func (p *Playout) UnmarshalYAML(value *yaml.Node) error {
	prev := *p

	type playoutYAML Playout
	if err := common.DecodeStrict(value, (*playoutYAML)(p)); err != nil {
		return err
	}

	p.TemplateVars = common.MergeNameValues(prev.TemplateVars, p.TemplateVars)
	p.EnvVars = common.MergeNameValues(prev.EnvVars, p.EnvVars)

	return nil
}

func (p *Playout) Validate() error {
	for i, templateVar := range p.TemplateVars {
		if err := templateVar.Validate(); err != nil {
			return fmt.Errorf("template_variables[%d]: %w", i, err)
		}
		if shell.IsReservedVar(templateVar.Name) {
			return fmt.Errorf("template_variables[%d]: %q is a reserved variable", i, templateVar.Name)
		}
	}
	for i, envVar := range p.EnvVars {
		if err := envVar.Validate(); err != nil {
			return fmt.Errorf("env_variables[%d]: %w", i, err)
		}
	}
	if p.InitSegments < 0 {
		return fmt.Errorf("init_segments cannot be negative")
	}
	if p.Order != "" && !validPlayoutOrders[p.Order] {
		return fmt.Errorf("order must be one of sequential, shuffle, interleave")
	}
	if p.ScheduleSwapAt != "" {
		if _, _, err := ParsePlayoutSwapAt(p.ScheduleSwapAt); err != nil {
			return fmt.Errorf("schedule_swap_at: %w", err)
		}
	}
	return nil
}

func (p *Playout) ResolvedRefreshInterval() time.Duration {
	if p.RefreshInterval == nil {
		return 0
	}
	return time.Duration(*p.RefreshInterval)
}

func (p *Playout) ResolvedScheduleSwapAt() (hour, minute int) {
	h, m, err := ParsePlayoutSwapAt(p.ScheduleSwapAt)
	if err != nil {
		return 0, 0
	}
	return h, m
}

func ParsePlayoutSwapAt(v string) (hour, minute int, err error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("must be in HH:MM format, got %q", v)
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour must be 0-23, got %q", v)
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("minute must be 0-59, got %q", v)
	}
	return hour, minute, nil
}
