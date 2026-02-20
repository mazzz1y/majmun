package channel

import (
	"bytes"
	"context"
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/config/rules/channel"
	"majmun/internal/listing/m3u8/store"
	"net/url"
	"slices"
)

type Processor struct {
	clientName string
	rules      []*channel.Rule
}

func NewRulesProcessor(clientName string, rules []*channel.Rule) *Processor {
	return &Processor{
		clientName: clientName,
		rules:      rules,
	}
}

func (p *Processor) Apply(ctx context.Context, store *store.Store) error {
	for _, ch := range store.All() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for i, rule := range p.rules {
			if err := p.processChannelRule(ch, rule); err != nil {
				return fmt.Errorf("channel_rule[%d]: %w", i, err)
			}
		}
	}
	return nil
}

func (p *Processor) processChannelRule(ch *store.Channel, rule *channel.Rule) error {
	switch {
	case rule.SetField != nil:
		return p.processSetField(ch, rule.SetField)
	case rule.RemoveField != nil:
		p.processRemoveField(ch, rule.RemoveField)
	case rule.RemoveChannel != nil:
		p.processRemoveChannel(ch, rule.RemoveChannel)
	case rule.MarkHidden != nil:
		p.processMarkHidden(ch, rule.MarkHidden)
	}
	return nil
}

func (p *Processor) processSetField(ch *store.Channel, rule *channel.SetFieldRule) error {
	if rule.Condition != nil && !p.matchesCondition(ch, *rule.Condition) {
		return nil
	}

	pl := ch.Playlist()

	var channelURL string
	if ch.URI() != nil {
		channelURL = ch.URI().String()
	}

	tmplMap := map[string]any{
		"Channel": map[string]any{
			"Name":  ch.Name(),
			"URL":   channelURL,
			"Attrs": ch.Attrs(),
			"Tags":  ch.Tags(),
		},
		"Playlist": map[string]any{
			"Name":      pl.Name(),
			"IsProxied": pl.IsProxied(),
		},
	}
	var buf bytes.Buffer

	if err := rule.Template.ToTemplate().Execute(&buf, tmplMap); err != nil {
		return err
	}

	value := buf.String()
	switch rule.Selector.Type {
	case common.SelectorName:
		ch.SetName(value)
	case common.SelectorURL:
		u, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("set_field: invalid URL '%s': %w", value, err)
		}
		ch.SetURI(u)
	case common.SelectorAttr:
		ch.SetAttr(rule.Selector.Value, value)
	case common.SelectorTag:
		ch.SetTag(rule.Selector.Value, value)
	}
	return nil
}

func (p *Processor) processRemoveField(ch *store.Channel, rule *channel.RemoveFieldRule) {
	if rule.Condition != nil && !p.matchesCondition(ch, *rule.Condition) {
		return
	}

	switch rule.Selector.Type {
	case common.SelectorAttr:
		for attrKey := range ch.Attrs() {
			if attrKey == rule.Selector.Value {
				ch.DeleteAttr(attrKey)
				break
			}
		}
	case common.SelectorTag:
		for tagKey := range ch.Tags() {
			if tagKey == rule.Selector.Value {
				ch.DeleteAttr(tagKey)
				break
			}
		}
	}
}

func (p *Processor) processRemoveChannel(ch *store.Channel, rule *channel.RemoveChannelRule) {
	if rule.Condition != nil && !p.matchesCondition(ch, *rule.Condition) {
		return
	}
	ch.MarkRemoved()
}

func (p *Processor) processMarkHidden(ch *store.Channel, rule *channel.MarkHiddenRule) {
	if rule.Condition != nil && !p.matchesCondition(ch, *rule.Condition) {
		return
	}
	ch.MarkHidden()
}

func (p *Processor) matchesCondition(ch *store.Channel, condition common.Condition) bool {
	if ch.IsRemoved() {
		return false
	}
	if condition.IsEmpty() {
		return true
	}

	fieldResult := p.evaluateField(ch, condition)

	var result bool
	if len(condition.And) > 0 {
		result = fieldResult && p.evaluateAnd(ch, condition.And)
	} else if len(condition.Or) > 0 {
		result = fieldResult && p.evaluateOr(ch, condition.Or)
	} else {
		result = fieldResult
	}

	if condition.Invert {
		result = !result
	}

	return result
}

func (p *Processor) evaluateField(ch *store.Channel, condition common.Condition) bool {
	hasFieldConditions := condition.Selector != nil || len(condition.Patterns) > 0 ||
		len(condition.Clients) > 0 || len(condition.Playlists) > 0

	if !hasFieldConditions {
		return true
	}

	if len(condition.Patterns) > 0 {
		fieldValue, ok := ch.GetFieldValue(condition.Selector)
		if !ok {
			return false
		}
		if !p.matchesRegexps(fieldValue, condition.Patterns) {
			return false
		}
	}

	if len(condition.Clients) > 0 && !slices.Contains([]string(condition.Clients), p.clientName) {
		return false
	}

	if len(condition.Playlists) > 0 && !slices.Contains([]string(condition.Playlists), ch.Playlist().Name()) {
		return false
	}

	return true
}

func (p *Processor) evaluateAnd(ch *store.Channel, conditions []common.Condition) bool {
	for _, sub := range conditions {
		if !p.matchesCondition(ch, sub) {
			return false
		}
	}
	return true
}

func (p *Processor) evaluateOr(ch *store.Channel, conditions []common.Condition) bool {
	for _, sub := range conditions {
		if p.matchesCondition(ch, sub) {
			return true
		}
	}
	return false
}

func (p *Processor) matchesRegexps(value string, regexps common.RegexpArr) bool {
	for _, re := range regexps {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}
