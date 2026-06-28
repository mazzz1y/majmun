package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/shell"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	orderSequential = "sequential"
	orderShuffle    = "shuffle"
	orderInterleave = "interleave"
	orderSpread     = "spread"
)

var validPlayoutOrders = []string{orderSequential, orderShuffle, orderInterleave, orderSpread}

var validFillerOrders = []string{orderSequential, orderShuffle}

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
	SeasonPatterns  common.RegexpArr   `yaml:"season_patterns,omitempty"`
	EpisodePatterns common.RegexpArr   `yaml:"episode_patterns,omitempty"`
	RefreshInterval *common.Duration   `yaml:"refresh_interval,omitempty"`
	EPGDuration     common.Duration    `yaml:"epg_duration,omitempty"`
	ScheduleSwapAt  string             `yaml:"schedule_swap_at,omitempty"`
	Metadata        PlayoutMetadata    `yaml:"metadata,omitempty"`
	Filler          PlayoutFiller      `yaml:"filler,omitempty"`
}

type PlayoutMetadata struct {
	Title       *common.Template `yaml:"title,omitempty"`
	Description *common.Template `yaml:"description,omitempty"`
	Category    *common.Template `yaml:"category,omitempty"`
}

type PlayoutFiller struct {
	Sources     common.StringOrArr `yaml:"sources,omitempty"`
	EveryCount  int                `yaml:"every_count,omitempty"`
	Every       common.Duration    `yaml:"every,omitempty"`
	MaxDuration common.Duration    `yaml:"max_duration,omitempty"`
	Order       string             `yaml:"order,omitempty"`
	Metadata    PlayoutMetadata    `yaml:"metadata,omitempty"`
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
	if p.Order != "" && !slices.Contains(validPlayoutOrders, p.Order) {
		return fmt.Errorf("order must be one of %s", strings.Join(validPlayoutOrders, ", "))
	}
	if p.ScheduleSwapAt != "" {
		if _, _, err := ParsePlayoutSwapAt(p.ScheduleSwapAt); err != nil {
			return fmt.Errorf("schedule_swap_at: %w", err)
		}
	}
	if err := p.Filler.Validate(); err != nil {
		return fmt.Errorf("filler: %w", err)
	}
	return nil
}

func (a *PlayoutFiller) Validate() error {
	if a.Order != "" && !slices.Contains(validFillerOrders, a.Order) {
		return fmt.Errorf("order must be one of %s", strings.Join(validFillerOrders, ", "))
	}
	if len(a.Sources) == 0 {
		if a.EveryCount != 0 || a.Every != 0 || a.MaxDuration != 0 {
			return fmt.Errorf("sources is required when filler is configured")
		}
		return nil
	}
	if a.EveryCount != 0 && a.Every != 0 {
		return fmt.Errorf("set only one of every_count or every")
	}
	if a.EveryCount < 0 {
		return fmt.Errorf("every_count must be greater than 0")
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
