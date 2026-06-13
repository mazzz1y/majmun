package proxy

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/shell"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var defaultPlayoutExtensions = []string{
	"mkv", "mp4", "avi", "mov", "m4v", "ts", "webm", "mpg", "mpeg", "flv", "wmv"}

const (
	defaultPlayoutRefresh     = 30 * time.Minute
	defaultPlayoutEPGDuration = 7 * 24 * time.Hour
	defaultPlayoutSwapHour    = 4
	defaultPlayoutSwapMinute  = 0
	defaultPlayoutStateDir    = "state"
)

// Playout configures generated channels: the FFmpeg transcode that turns local media into a
// continuous HLS stream, plus the scheduling and listing settings around it. It cascades
// global -> playlist -> channel, so shared settings are defined once and a channel only
// overrides what differs. Transcode defaults come from DefaultConfig; the scheduling/listing
// fields are pointers resolved lazily, so an unset level inherits instead of overwriting.
type Playout struct {
	Command      common.StringOrArr `yaml:"command,omitempty"`
	TemplateVars []common.NameValue `yaml:"template_variables,omitempty"`
	EnvVars      []common.NameValue `yaml:"env_variables,omitempty"`
	InitSegments *int               `yaml:"init_segments,omitempty"`
	ReadyTimeout *common.Duration   `yaml:"ready_timeout,omitempty"`

	StateDir        string             `yaml:"state_dir,omitempty"`
	Logo            string             `yaml:"logo,omitempty"`
	Extensions      common.StringOrArr `yaml:"extensions,omitempty"`
	RandomOrder     *bool              `yaml:"random_order,omitempty"`
	RefreshInterval *common.Duration   `yaml:"refresh_interval,omitempty"`
	EPGDuration     *common.Duration   `yaml:"epg_duration,omitempty"`
	ScheduleSwapAt  *string            `yaml:"schedule_swap_at,omitempty"`
}

func (p *Playout) GetCommand() common.StringOrArr      { return p.Command }
func (p *Playout) GetEnvVars() []common.NameValue      { return p.EnvVars }
func (p *Playout) GetTemplateVars() []common.NameValue { return p.TemplateVars }
func (p *Playout) GetInitSegments() *int               { return p.InitSegments }
func (p *Playout) GetReadyTimeout() *common.Duration   { return p.ReadyTimeout }

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
	if p.InitSegments != nil && *p.InitSegments < 1 {
		return fmt.Errorf("init_segments must be at least 1")
	}
	if p.ScheduleSwapAt != nil {
		if _, _, err := parseSwapAt(*p.ScheduleSwapAt); err != nil {
			return fmt.Errorf("schedule_swap_at: %w", err)
		}
	}
	return nil
}

func (p *Playout) ResolvedStateDir() string {
	if p.StateDir == "" {
		return defaultPlayoutStateDir
	}
	return p.StateDir
}

func (p *Playout) ResolvedExtensions() []string {
	if len(p.Extensions) > 0 {
		return p.Extensions
	}
	return defaultPlayoutExtensions
}

func (p *Playout) ResolvedRandomOrder() bool {
	return p.RandomOrder != nil && *p.RandomOrder
}

func (p *Playout) ResolvedRefreshInterval() time.Duration {
	if p.RefreshInterval == nil {
		return defaultPlayoutRefresh
	}
	return time.Duration(*p.RefreshInterval)
}

func (p *Playout) ResolvedEPGDuration() time.Duration {
	if p.EPGDuration == nil {
		return defaultPlayoutEPGDuration
	}
	return time.Duration(*p.EPGDuration)
}

func (p *Playout) ResolvedScheduleSwapAt() (hour, minute int) {
	if p.ScheduleSwapAt == nil {
		return defaultPlayoutSwapHour, defaultPlayoutSwapMinute
	}
	hour, minute, err := parseSwapAt(*p.ScheduleSwapAt)
	if err != nil {
		return defaultPlayoutSwapHour, defaultPlayoutSwapMinute
	}
	return hour, minute
}

func parseSwapAt(v string) (hour, minute int, err error) {
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
