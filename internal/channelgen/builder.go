package channelgen

import (
	"context"
	"majmun/internal/logging"
	"majmun/internal/natsort"
	"math/rand"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func buildSchedule(ctx context.Context, p prober, id string, sources, extensions []string, order string, seasonPatterns, episodePatterns []*regexp.Regexp, old *Schedule, now time.Time) (*Schedule, error) {
	files, err := scanSources(sources, extensions)
	if err != nil {
		return nil, err
	}

	fp := fingerprint(files, order)
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
		season, episode := res.Season, res.Episode
		if episode == 0 {
			season, episode = parseEpisode(res.EpisodeTag, episodePatterns)
		}
		if episode == 0 {
			// Tag carries no episode; fall back to the filename.
			season, episode = parseEpisode(titleFromPath(f.path), episodePatterns)
		}
		if season == 0 {
			// Neither tag nor filename carry a season; fall back to the season folder.
			season = seasonFromPath(f.path, sources, seasonPatterns)
		}
		items = append(items, Item{
			File:           f.path,
			Title:          res.Title,
			Show:           res.Show,
			Description:    res.Description,
			Category:       res.Category,
			Date:           res.Date,
			Season:         season,
			Episode:        episode,
			Size:           f.size,
			MTime:          f.mtime,
			Duration:       res.Duration,
			VideoCodec:     res.VideoCodec,
			Width:          res.Width,
			Height:         res.Height,
			AspectWidth:    res.AspectWidth,
			PixelFormat:    res.PixelFormat,
			FrameRate:      res.FrameRate,
			FieldOrder:     res.FieldOrder,
			AudioCodec:     res.AudioCodec,
			AudioChannels:  res.AudioChannels,
			SampleRate:     res.SampleRate,
			AudioLanguages: res.AudioLanguages,
		})
	}

	seed := int64(0)
	if old != nil {
		seed = old.Seed
	}
	switch order {
	case "shuffle":
		if old == nil || old.Seed == 0 {
			seed = now.UnixNano()
		}
		shuffle(items, seed)
	case "interleave":
		items = interleave(items, sources, seasonPatterns)
	case "spread":
		items = spread(items, sources, seasonPatterns)
	default:
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

// seriesKey identifies the show a file belongs to: the path above the season segment, relative
// to its source ("season" matched by seasonPatterns). When that prefix is empty, the show name
// itself looks like a season, so the cut falls to the deepest season segment instead. Examples
// (source "/s"):
//
//	/s/Show/Season 1/ep      -> Show
//	/s/Show/ep               -> Show
//	/s/Season 25/Season 1/ep -> Season 25   (leading name kept)
//	/s/ep                    -> s           (source name; not a series of one)
func seriesKey(file string, sources []string, seasonPatterns []*regexp.Regexp) string {
	rel, source := relToSource(file, sources)
	segs := strings.Split(rel, string(filepath.Separator))
	dirs := segs[:len(segs)-1]

	firstSeason, lastSeason := -1, -1
	for i, seg := range dirs {
		if isSeasonDir(seg, seasonPatterns) {
			if firstSeason == -1 {
				firstSeason = i
			}
			lastSeason = i
		}
	}

	if firstSeason == -1 {
		if len(dirs) == 0 && source != "" {
			return filepath.Base(source)
		}
		return segs[0]
	}

	cut := firstSeason
	if cut == 0 {
		cut = lastSeason
	}
	if cut == 0 {
		return filepath.Base(source)
	}
	return strings.Join(dirs[:cut], string(filepath.Separator))
}

var seasonNumRegexp = regexp.MustCompile(`\d+`)

// seasonFromPath extracts the season number from the deepest season folder on a file's path
// relative to its source ("Сезон 2"/"Season 02"/"S3" -> 2/2/3), or 0 if none is present. Used
// to fill Season when tags and the filename carry no season.
func seasonFromPath(file string, sources []string, seasonPatterns []*regexp.Regexp) int {
	rel, _ := relToSource(file, sources)
	dirs := strings.Split(rel, string(filepath.Separator))
	dirs = dirs[:len(dirs)-1]
	for i := len(dirs) - 1; i >= 0; i-- {
		if isSeasonDir(dirs[i], seasonPatterns) {
			if m := seasonNumRegexp.FindString(dirs[i]); m != "" {
				n, _ := strconv.Atoi(m)
				return n
			}
		}
	}
	return 0
}

func isSeasonDir(name string, seasonPatterns []*regexp.Regexp) bool {
	name = strings.TrimSpace(name)
	for _, re := range seasonPatterns {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// relToSource returns the file's path relative to the source root that contains it, plus that
// root. A file under no source falls back to its bare filename and an empty root.
func relToSource(file string, sources []string) (rel, source string) {
	for _, src := range sources {
		r, err := filepath.Rel(src, file)
		if err != nil || strings.HasPrefix(r, "..") {
			continue
		}
		return r, src
	}
	return filepath.Base(file), ""
}

// groupShows buckets items by show (container tag, else season-aware path), sorting each
// show's episodes by episode order and returning the shows ordered by name (natsort).
func groupShows(items []Item, sources []string, seasonPatterns []*regexp.Regexp) [][]Item {
	groups := map[string][]Item{}
	for _, it := range items {
		k := it.Show
		if k == "" {
			k = seriesKey(it.File, sources, seasonPatterns)
		}
		groups[k] = append(groups[k], it)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return natsort.Less(keys[i], keys[j]) })

	shows := make([][]Item, len(keys))
	for i, k := range keys {
		g := groups[k]
		sort.Slice(g, func(a, b int) bool { return itemLess(g[a], g[b]) })
		shows[i] = g
	}
	return shows
}

// interleave round-robins episodes across shows: each show is sorted by episode order, then
// position k from every show is taken in turn. Shorter shows drop out as they run out.
func interleave(items []Item, sources []string, seasonPatterns []*regexp.Regexp) []Item {
	shows := groupShows(items, sources, seasonPatterns)
	out := make([]Item, 0, len(items))
	for round := 0; len(out) < len(items); round++ {
		for _, g := range shows {
			if round < len(g) {
				out = append(out, g[round])
			}
		}
	}
	return out
}

// spread distributes each show's episodes evenly across the whole timeline: episode j of a
// show with n episodes is placed at position (j+0.5)/n in [0,1). Items are then ordered by
// that position, so a long show and a short show both run start-to-finish, the long one simply
// appearing more often. Ties keep show order (shows are pre-sorted by name).
func spread(items []Item, sources []string, seasonPatterns []*regexp.Regexp) []Item {
	shows := groupShows(items, sources, seasonPatterns)

	type placed struct {
		pos  float64
		show int
		it   Item
	}
	all := make([]placed, 0, len(items))
	for s, g := range shows {
		n := len(g)
		for j, it := range g {
			all = append(all, placed{pos: (float64(j) + 0.5) / float64(n), show: s, it: it})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].pos != all[j].pos {
			return all[i].pos < all[j].pos
		}
		return all[i].show < all[j].show
	})

	out := make([]Item, len(all))
	for i, p := range all {
		out[i] = p.it
	}
	return out
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
