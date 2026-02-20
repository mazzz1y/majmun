package playlist

import (
	"context"
	"fmt"
	"majmun/internal/config/common"
	playlistconf "majmun/internal/config/rules/playlist"
	"majmun/internal/listing/m3u8/store"
	"slices"
)

type Processor struct {
	rules      []*playlistconf.Rule
	clientName string
}

func NewRulesProcessor(clientName string, rules []*playlistconf.Rule) *Processor {
	return &Processor{
		rules:      rules,
		clientName: clientName,
	}
}

func (p *Processor) Apply(ctx context.Context, store *store.Store) error {
	var err error
	for i, rule := range p.rules {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if rule.MergeChannels != nil && evaluateStoreCondition(rule.MergeChannels.Condition, p.clientName) {
			processor := NewMergeDuplicatesActionProcessor(rule.MergeChannels)
			err = processor.Apply(store)
		}
		if rule.RemoveDuplicates != nil && evaluateStoreCondition(rule.RemoveDuplicates.Condition, p.clientName) {
			processor := NewRemoveDuplicatesActionProcessor(rule.RemoveDuplicates)
			err = processor.Apply(store)
		}
		if rule.SortRule != nil && evaluateStoreCondition(rule.SortRule.Condition, p.clientName) {
			processor := NewSortProcessor(rule.SortRule)
			processor.Apply(store)
		}
		if err != nil {
			return fmt.Errorf("playlist_rules[%d]: %s", i, err)
		}
	}
	return nil
}

func evaluateStoreCondition(condition *common.Condition, clientName string) bool {
	if condition == nil {
		return true
	}

	if len(condition.Clients) > 0 {
		if slices.Contains([]string(condition.Clients), clientName) {
			return !condition.Invert
		}
		return condition.Invert
	}

	return true
}
