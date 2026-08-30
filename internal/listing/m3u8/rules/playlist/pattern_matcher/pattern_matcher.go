package pattern_matcher

import (
	"cmp"
	"majmun/internal/listing/m3u8/store"
	"regexp"
	"slices"
	"strings"

	"majmun/internal/config/common"
)

type PatternMatcher struct {
	channels     []*store.Channel
	selector     *common.Selector
	patternArray []*regexp.Regexp
	channelPrios map[*store.Channel][]int
	emptyPrio    int
}

func NewPatternMatcher(channels []*store.Channel, selector *common.Selector, patterns common.RegexpArr) *PatternMatcher {
	return &PatternMatcher{
		channels:     channels,
		selector:     selector,
		patternArray: patterns.ToArray(),
	}
}

func (pm *PatternMatcher) GroupChannels() map[string][]*store.Channel {
	groups := make(map[string][]*store.Channel)

	allGroups := make(map[string][]*store.Channel)
	for _, ch := range pm.channels {
		baseName, _ := pm.extractBaseName(ch)
		allGroups[baseName] = append(allGroups[baseName], ch)
	}

	for baseName, channels := range allGroups {
		hasModified := false
		for _, ch := range channels {
			fv, ok := ch.GetFieldValue(pm.selector)
			if !ok {
				continue
			}
			if baseName != fv {
				hasModified = true
				break
			}
		}
		if hasModified && len(channels) > 1 {
			groups[baseName] = channels
		}
	}

	for _, channels := range groups {
		pm.matchGroup(channels)
		slices.SortFunc(channels, func(a, b *store.Channel) int {
			return cmp.Compare(b.Priority(), a.Priority())
		})
	}

	return groups
}

func (pm *PatternMatcher) matchGroup(channels []*store.Channel) {
	if len(pm.patternArray) == 0 {
		return
	}

	pm.computePriorities(channels)
	pm.setPriorities()
}

func (pm *PatternMatcher) computePriorities(channels []*store.Channel) {
	pm.channelPrios = make(map[*store.Channel][]int)
	priority := len(pm.patternArray)

	for _, pattern := range pm.patternArray {
		patternStr := pattern.String()
		if patternStr == "" {
			pm.emptyPrio = priority
		}
		for _, ch := range channels {
			fv, ok := ch.GetFieldValue(pm.selector)
			if !ok || fv == "" {
				continue
			}
			if pattern.MatchString(fv) {
				pm.channelPrios[ch] = append(pm.channelPrios[ch], priority)
			}
		}
		priority--
	}
}

func (pm *PatternMatcher) setPriorities() {
	for ch, prios := range pm.channelPrios {
		maxNonEmpty := -1
		for _, p := range prios {
			if p != pm.emptyPrio {
				maxNonEmpty = max(maxNonEmpty, p)
			}
		}
		if maxNonEmpty >= 0 {
			ch.SetPriority(maxNonEmpty)
		} else {
			ch.SetPriority(pm.emptyPrio)
		}
	}
}

func (pm *PatternMatcher) extractBaseName(ch *store.Channel) (string, bool) {
	fv, ok := ch.GetFieldValue(pm.selector)
	if !ok {
		return "", false
	}

	if fv == "" {
		return "", true
	}

	if len(pm.patternArray) == 0 {
		return fv, true
	}

	result := fv
	for _, pattern := range pm.patternArray {
		result = pattern.ReplaceAllString(result, "")
	}

	return strings.Join(strings.Fields(result), " "), true
}
