package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/shell"
	"strconv"
	"strings"
	"time"
)

var defaultChannelExtensions = []string{
	"mkv", "mp4", "avi", "mov", "m4v", "ts", "webm", "mpg", "mpeg", "flv", "wmv"}

const (
	defaultChannelRefresh     = 5 * time.Minute
	defaultChannelEPGDuration = 7 * 24 * time.Hour
	defaultScheduleSwapHour   = 4
	defaultScheduleSwapMinute = 0
)

type Channel struct {
	Name            string             `yaml:"name"`
	Logo            string             `yaml:"logo,omitempty"`
	Fields          []ChannelField     `yaml:"fields,omitempty"`
	TemplateVars    []common.NameValue `yaml:"template_variables,omitempty"`
	Sources         common.StringOrArr `yaml:"sources"`
	RandomOrder     bool               `yaml:"random_order,omitempty"`
	Extensions      common.StringOrArr `yaml:"extensions,omitempty"`
	RefreshInterval *common.Duration   `yaml:"refresh_interval,omitempty"`
	EPGDuration     *common.Duration   `yaml:"epg_duration,omitempty"`
	ScheduleSwapAt  *string            `yaml:"schedule_swap_at,omitempty"`
}

type ChannelField struct {
	Selector *common.Selector `yaml:"selector"`
	Template *common.Template `yaml:"template"`
}

func (f *ChannelField) Validate() error {
	if f.Selector == nil {
		return fmt.Errorf("selector is required")
	}
	if err := f.Selector.Validate(); err != nil {
		return err
	}
	if f.Selector.Type != common.SelectorAttr && f.Selector.Type != common.SelectorTag {
		return fmt.Errorf("selector must be attr/<key> or tag/<key>, got '%s'", f.Selector.Raw)
	}
	if f.Template == nil {
		return fmt.Errorf("template is required")
	}
	return nil
}

func (c *Channel) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("sources is required")
	}
	for i, source := range c.Sources {
		if source == "" {
			return fmt.Errorf("sources[%d] cannot be empty", i)
		}
	}
	for i, field := range c.Fields {
		if err := field.Validate(); err != nil {
			return fmt.Errorf("fields[%d]: %w", i, err)
		}
	}
	for i, templateVar := range c.TemplateVars {
		if err := templateVar.Validate(); err != nil {
			return fmt.Errorf("template_variables[%d]: %w", i, err)
		}
		if shell.IsReservedVar(templateVar.Name) {
			return fmt.Errorf("template_variables[%d]: %q is a reserved variable", i, templateVar.Name)
		}
	}
	if c.ScheduleSwapAt != nil {
		if _, _, err := parseSwapAt(*c.ScheduleSwapAt); err != nil {
			return fmt.Errorf("schedule_swap_at: %w", err)
		}
	}
	return nil
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

func (c *Channel) ResolvedExtensions() []string {
	if len(c.Extensions) > 0 {
		return c.Extensions
	}
	return defaultChannelExtensions
}

func (c *Channel) ResolvedRefreshInterval() time.Duration {
	if c.RefreshInterval == nil {
		return defaultChannelRefresh
	}
	return time.Duration(*c.RefreshInterval)
}

func (c *Channel) ResolvedEPGDuration() time.Duration {
	if c.EPGDuration == nil {
		return defaultChannelEPGDuration
	}
	return time.Duration(*c.EPGDuration)
}

func (c *Channel) ResolvedScheduleSwapAt() (hour, minute int) {
	if c.ScheduleSwapAt == nil {
		return defaultScheduleSwapHour, defaultScheduleSwapMinute
	}
	hour, minute, err := parseSwapAt(*c.ScheduleSwapAt)
	if err != nil {
		return defaultScheduleSwapHour, defaultScheduleSwapMinute
	}
	return hour, minute
}
