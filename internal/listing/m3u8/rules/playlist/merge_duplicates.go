package playlist

import (
	"bytes"
	"majmun/internal/config/common"
	configrules "majmun/internal/config/rules/playlist"
	"majmun/internal/listing/m3u8/rules/playlist/pattern_matcher"
	"majmun/internal/listing/m3u8/store"
	"majmun/internal/parser/m3u8"
)

type MergeDuplicatesProcessor struct {
	rule    *configrules.MergeDuplicatesRule
	matcher interface {
		GroupChannels() map[string][]*store.Channel
	}
}

func NewMergeDuplicatesActionProcessor(rule *configrules.MergeDuplicatesRule) *MergeDuplicatesProcessor {
	return &MergeDuplicatesProcessor{rule: rule}
}

func (p *MergeDuplicatesProcessor) Apply(store *store.Store) error {
	if p.matcher == nil {
		p.matcher = pattern_matcher.NewPatternMatcher(store.All(), p.rule.Selector, p.rule.Patterns)
	}
	grouped := p.matcher.GroupChannels()
	return p.processMergeGroups(grouped)
}

func (p *MergeDuplicatesProcessor) processMergeGroups(groups map[string][]*store.Channel) error {
	for baseName, group := range groups {
		if len(group) <= 1 {
			continue
		}

		best := group[0]
		pl := best.Playlist()

		if bestTvgId, exists := best.GetAttr(m3u8.AttrTvgID); exists {
			for i := 1; i < len(group); i++ {
				group[i].SetAttr(m3u8.AttrTvgID, bestTvgId)
			}
		}

		if p.rule.FinalValue != nil {
			tmplMap := map[string]any{
				"Channel": map[string]any{
					"BaseName": baseName,
					"Name":     best.Name(),
					"Attrs":    best.Attrs(),
					"Tags":     best.Tags(),
				},
				"Playlist": map[string]any{
					"Name":      pl.Name(),
					"IsProxied": pl.IsProxied(),
				},
			}

			var buf bytes.Buffer
			if err := p.rule.FinalValue.Template.ToTemplate().Execute(&buf, tmplMap); err != nil {
				return err
			}
			finalValue := buf.String()

			for _, ch := range group {
				switch p.rule.FinalValue.Selector.Type {
				case common.SelectorName:
					ch.SetName(finalValue)
				case common.SelectorAttr:
					ch.SetAttr(p.rule.FinalValue.Selector.Value, finalValue)
				case common.SelectorTag:
					ch.SetTag(p.rule.FinalValue.Selector.Value, finalValue)
				}
			}

		}
	}
	return nil
}
