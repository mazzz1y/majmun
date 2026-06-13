package channelgen

import (
	"context"
	"majmun/internal/logging"
	"majmun/internal/natsort"
	"math/rand"
	"path/filepath"
	"sort"
	"time"
)

func buildSchedule(ctx context.Context, p prober, id string, sources, extensions []string, randomOrder bool, old *Schedule, now time.Time) (*Schedule, error) {
	files, err := scanSources(sources, extensions)
	if err != nil {
		return nil, err
	}

	fp := fingerprint(files, randomOrder)
	if old != nil && old.Fingerprint == fp {
		return old, nil
	}

	var cache map[probeKey]probeResult
	if old != nil {
		cache = old.probeCache()
	} else {
		cache = map[probeKey]probeResult{}
	}

	items := make([]Item, 0, len(files))
	for _, f := range files {
		cacheKey := probeKey{file: f.path, size: f.size, mtime: f.mtime}
		res, ok := cache[cacheKey]
		if !ok {
			res, err = p.Probe(ctx, f.path)
			if err != nil {
				logging.Error(ctx, err, "failed to probe file duration, excluding", "file", f.path)
				continue
			}
		}
		if res.Duration <= 0 {
			continue
		}
		title := res.Title
		if title == "" {
			title = titleFromPath(f.path)
		}
		season, episode := res.Season, res.Episode
		if episode == 0 {
			// Tags carry no episode info; fall back to filename patterns (S01E05, 1x05, ep5, ...).
			season, episode = parseEpisode(titleFromPath(f.path))
		}
		items = append(items, Item{
			File:        f.path,
			Title:       title,
			Description: res.Description,
			Category:    res.Category,
			Date:        res.Date,
			Season:      season,
			Episode:     episode,
			Size:        f.size,
			MTime:       f.mtime,
			Duration:    res.Duration,
		})
	}

	seed := int64(0)
	if old != nil {
		seed = old.Seed
	}
	if randomOrder {
		if old == nil || old.Seed == 0 {
			seed = now.UnixNano()
		}
		shuffle(items, seed)
	} else {
		sort.Slice(items, func(i, j int) bool {
			return itemLess(items[i], items[j])
		})
	}

	anchor := now.Unix()
	if old != nil && old.Anchor != 0 {
		anchor = old.Anchor
	}

	return &Schedule{
		Channel:     id,
		Seed:        seed,
		Fingerprint: fp,
		Anchor:      anchor,
		Items:       items,
	}, nil
}

// itemLess orders the schedule by the composite key (directory, season, episode,
// filename), each part compared naturally. Items without episode info carry 0/0 and
// keeping them in the composite key (rather than branching on presence) keeps the
// ordering transitive when tagged and untagged files share a directory.
func itemLess(a, b Item) bool {
	if da, db := filepath.Dir(a.File), filepath.Dir(b.File); da != db {
		return natsort.Less(da, db)
	}
	if a.Season != b.Season {
		return a.Season < b.Season
	}
	if a.Episode != b.Episode {
		return a.Episode < b.Episode
	}
	return natsort.Less(filepath.Base(a.File), filepath.Base(b.File))
}

func shuffle(items []Item, seed int64) {
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}

// locate uses pure-modulo wrap over the total cycle, so item order is fixed until the
// file set changes.
func locate(s *Schedule, now time.Time) (index int, item Item, offset float64, nextBoundary time.Time, ok bool) {
	if s.isEmpty() {
		return 0, Item{}, 0, time.Time{}, false
	}

	total := s.total()
	elapsed := mathMod(float64(now.Unix()-s.Anchor), total)

	cycleStart := now.Add(-time.Duration(elapsed * float64(time.Second)))

	var acc float64
	for i, it := range s.Items {
		if elapsed < acc+it.Duration {
			offset = elapsed - acc
			boundary := cycleStart.Add(time.Duration((acc + it.Duration) * float64(time.Second)))
			return i, it, offset, boundary, true
		}
		acc += it.Duration
	}

	// Floating point edge: land on the last item.
	lastIdx := len(s.Items) - 1
	last := s.Items[lastIdx]
	boundary := cycleStart.Add(time.Duration(total * float64(time.Second)))
	return lastIdx, last, last.Duration, boundary, true
}

// earliestRemovedAir returns the soonest next occurrence in old's looping timeline of a file
// present in old but absent from s, and whether any such file exists. The result lies in
// [now, now+total): a removed file that is airing right now reports its next loop occurrence,
// which is exactly when the running transcode would reopen (and fail on) the deleted file. It
// is used to decide whether a removal must be adopted before the daily swap.
func earliestRemovedAir(old, s *Schedule, now time.Time) (time.Time, bool) {
	total := old.total()
	if total <= 0 {
		return time.Time{}, false
	}

	present := make(map[string]struct{}, len(s.Items))
	for _, it := range s.Items {
		present[it.File] = struct{}{}
	}

	elapsed := mathMod(float64(now.Unix()-old.Anchor), total)

	var acc float64
	best := time.Time{}
	found := false
	for _, it := range old.Items {
		if _, ok := present[it.File]; !ok {
			delta := mathMod(acc-elapsed, total)
			air := now.Add(time.Duration(delta * float64(time.Second)))
			if !found || air.Before(best) {
				best, found = air, true
			}
		}
		acc += it.Duration
	}
	return best, found
}

func mathMod(a, m float64) float64 {
	if m <= 0 {
		return 0
	}
	r := a - m*float64(int64(a/m))
	if r < 0 {
		r += m
	}
	return r
}
