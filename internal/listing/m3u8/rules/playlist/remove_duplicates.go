package playlist

import (
	"bytes"
	"majmun/internal/config/common"
	"majmun/internal/config/rules/playlist"
	"majmun/internal/listing/m3u8/rules/playlist/pattern_matcher"
	"majmun/internal/listing/m3u8/store"
)

type RemoveDuplicatesProcessor struct {
	rule    *playlist.RemoveDuplicatesRule
	matcher interface {
		GroupChannels() map[string][]*store.Channel
	}
}

func NewRemoveDuplicatesActionProcessor(rule *playlist.RemoveDuplicatesRule) *RemoveDuplicatesProcessor {
	return &RemoveDuplicatesProcessor{rule: rule}
}

func (p *RemoveDuplicatesProcessor) Apply(store *store.Store) error {
	if p.matcher == nil {
		p.matcher = pattern_matcher.NewPatternMatcher(store.All(), p.rule.Selector, p.rule.Patterns)
	}
	grouped := p.matcher.GroupChannels()
	return p.processDuplicateGroups(grouped)
}

func (p *RemoveDuplicatesProcessor) processDuplicateGroups(groups map[string][]*store.Channel) error {
	for baseName, group := range groups {
		if len(group) <= 1 {
			continue
		}

		best := group[0]

		for _, ch := range group {
			pl := ch.Playlist()

			if ch == best {
				if p.rule.FinalValue != nil {
					tmplMap := map[string]any{
						"Channel": map[string]any{
							"BaseName": baseName,
							"Name":     ch.Name(),
							"Attrs":    ch.Attrs(),
							"Tags":     ch.Tags(),
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

					if p.rule.FinalValue.Selector != nil {
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
			} else {
				ch.MarkRemoved()
			}
		}
	}
	return nil
}
