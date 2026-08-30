package channelgen

import (
	"cmp"
	"context"
	"majmun/internal/logging"
	"majmun/internal/natsort"
	"maps"
	"math/rand"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type scanResult struct {
	files       []scannedFile
	fillerFiles []scannedFile
	fingerprint string
}

func scanContent(sources, extensions []string, order Order, seasonPatterns, episodePatterns []*regexp.Regexp, filler FillerConfig) (scanResult, error) {
	files, err := scanSources(sources, extensions)
	if err != nil {
		return scanResult{}, err
	}
	fillerFiles, err := scanSources(filler.Sources, extensions)
	if err != nil {
		return scanResult{}, err
	}
	return scanResult{
		files:       files,
		fillerFiles: fillerFiles,
		fingerprint: fingerprint(files, fillerFiles, order, seasonPatterns, episodePatterns, filler),
	}, nil
}

func buildSchedule(ctx context.Context, p prober, id string, scan scanResult, sources []string, order Order, seasonPatterns, episodePatterns []*regexp.Regexp, filler FillerConfig, old *Schedule, now time.Time) *Schedule {
	files, fillerFiles, fp := scan.files, scan.fillerFiles, scan.fingerprint

	var cache map[probeKey]probeResult
	if old != nil {
		cache = old.probeCache()
	} else {
		cache = map[probeKey]probeResult{}
	}

	items := make([]Item, 0, len(files))
	for _, f := range files {
		res, ok := probeFile(ctx, p, cache, f)
		if !ok {
			continue
		}
		season, episode := deriveEpisode(f.path, res.EpisodeTag, res.Season, res.Episode, sources, seasonPatterns, episodePatterns)
		it := newItem(f, res)
		it.Season, it.Episode = season, episode
		items = append(items, it)
	}

	fillerPool := make([]Item, 0, len(fillerFiles))
	for _, f := range fillerFiles {
		res, ok := probeFile(ctx, p, cache, f)
		if !ok {
			continue
		}
		it := newItem(f, res)
		it.IsFiller = true
		fillerPool = append(fillerPool, it)
	}

	seed := int64(0)
	if old != nil {
		seed = old.Seed
	}
	if order == OrderShuffle && seed == 0 {
		seed = now.UnixNano()
	}

	fillerSeed := int64(0)
	if old != nil {
		fillerSeed = old.FillerSeed
	}
	if filler.Order == OrderShuffle && fillerSeed == 0 {
		fillerSeed = now.UnixNano() + 1
	}

	items = orderItems(items, order, sources, seasonPatterns, seed)
	fillerPool = orderItems(fillerPool, filler.Order, filler.Sources, nil, fillerSeed)

	anchor := now.Unix()
	if old != nil && old.Anchor != 0 {
		anchor = old.Anchor
	}

	fillerStart := 0
	if old != nil && len(fillerPool) > 0 {
		fillerStart = old.FillerStart % len(fillerPool)
	}
	withFiller, nextFillerStart := injectFiller(
		items, fillerPool, fillerStart, filler.EveryCount, filler.Every, filler.MaxDuration)

	return &Schedule{
		Channel:     id,
		Seed:        seed,
		Fingerprint: fp,
		Anchor:      anchor,
		Items:       withFiller,
		FillerSeed:  fillerSeed,
		FillerStart: nextFillerStart,
	}
}

// probeFile reads f from the cache or probes it, reporting ok=false (and logging) when it
// can't be probed or has no duration, so the caller skips it.
func probeFile(ctx context.Context, p prober, cache map[probeKey]probeResult, f scannedFile) (probeResult, bool) {
	cacheKey := probeKey{file: f.path, size: f.size, mtime: f.mtime}
	res, ok := cache[cacheKey]
	if !ok {
		var err error
		res, err = p.Probe(ctx, f.path)
		if err != nil {
			logging.Error(ctx, err, "failed to probe file duration, excluding", "file", f.path)
			return probeResult{}, false
		}
		cache[cacheKey] = res
	}
	if res.Duration <= 0 {
		return probeResult{}, false
	}
	return res, true
}

func newItem(f scannedFile, res probeResult) Item {
	return Item{
		File:           f.path,
		Title:          res.Title,
		Show:           res.Show,
		Description:    res.Description,
		Category:       res.Category,
		Date:           res.Date,
		EpisodeTag:     res.EpisodeTag,
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
	}
}

const defaultFillerInterval = time.Hour

// injectFiller inserts filler breaks between content, on a count (everyCount) or content-playtime
// (every) cadence, defaulting to defaultFillerInterval when neither is set. No break follows the
// final item. Clips are drawn from pool starting at start (cycling), and the pool index to
// resume from next time is returned so successive rebuilds rotate through the whole pool.
func injectFiller(content, pool []Item, start, everyCount int, every, maxDuration time.Duration) ([]Item, int) {
	if len(pool) == 0 {
		return content, start
	}
	if everyCount <= 0 && every <= 0 {
		every = defaultFillerInterval
	}

	out := make([]Item, 0, len(content)+len(content)/max(everyCount, 1))
	next := start
	var sinceBreak time.Duration
	for i, it := range content {
		out = append(out, it)
		sinceBreak += time.Duration(it.Duration * float64(time.Second))

		var breakNow bool
		if everyCount > 0 {
			breakNow = (i+1)%everyCount == 0
		} else {
			breakNow = sinceBreak >= every
		}
		if i == len(content)-1 || !breakNow {
			continue
		}
		out, next = fillBreak(out, pool, next, maxDuration)
		sinceBreak = 0
	}
	return out, next % len(pool)
}

// fillBreak appends clips from pool (cycling from next) up to maxDuration; the first clip is
// always placed, so a zero maxDuration yields one clip. It returns the advanced pool index.
func fillBreak(out, pool []Item, next int, maxDuration time.Duration) ([]Item, int) {
	maxSec := maxDuration.Seconds()
	var breakSec float64
	for {
		clip := pool[next%len(pool)]
		if breakSec > 0 && (maxSec <= 0 || breakSec+clip.Duration > maxSec) {
			break
		}
		out = append(out, clip)
		next++
		breakSec += clip.Duration
	}
	return out, next
}

func deriveEpisode(file, episodeTag string, probeSeason, probeEpisode int, sources []string, seasonPatterns, episodePatterns []*regexp.Regexp) (season, episode int) {
	season, episode = probeSeason, probeEpisode
	if episode == 0 {
		season, episode = parseEpisode(episodeTag, episodePatterns)
	}
	if episode == 0 {
		season, episode = parseEpisode(titleFromPath(file), episodePatterns)
	}
	if season == 0 {
		season = seasonFromPath(file, sources, seasonPatterns)
	}
	return season, episode
}

func orderItems(items []Item, order Order, sources []string, seasonPatterns []*regexp.Regexp, seed int64) []Item {
	switch order {
	case OrderShuffle:
		return shuffle(items, seed)
	case OrderInterleave:
		return interleave(items, sources, seasonPatterns)
	case OrderSpread:
		return spread(items, sources, seasonPatterns)
	default:
		return sortNatural(items)
	}
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

func groupShows(items []Item, sources []string, seasonPatterns []*regexp.Regexp) [][]Item {
	groups := map[string][]Item{}
	for _, it := range items {
		k := it.Show
		if k == "" {
			k = seriesKey(it.File, sources, seasonPatterns)
		}
		groups[k] = append(groups[k], it)
	}
	keys := slices.Collect(maps.Keys(groups))
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
	slices.SortStableFunc(all, func(a, b placed) int {
		return cmp.Or(cmp.Compare(a.pos, b.pos), cmp.Compare(a.show, b.show))
	})

	out := make([]Item, len(all))
	for i, p := range all {
		out[i] = p.it
	}
	return out
}

func sortNatural(items []Item) []Item {
	sort.Slice(items, func(i, j int) bool {
		return itemLess(items[i], items[j])
	})
	return items
}

func shuffle(items []Item, seed int64) []Item {
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
	return items
}

// locate reports which item plays at now, the offset into it (seconds), and when it ends, over a
// modulo wrap of the total cycle anchored at s.Anchor. The last item is an unconditional catch-all
// so float drift near the cycle end can't fall through to "no item"; the boundary is floored one
// nanosecond past now so the entry re-clock and EPG end never land on an already-consumed slot.
func locate(s *Schedule, now time.Time) (int, Item, float64, time.Time, bool) {
	if s.isEmpty() {
		return 0, Item{}, 0, time.Time{}, false
	}

	total := s.total()
	nowSec := float64(now.UnixNano()) / float64(time.Second)
	elapsed := mathMod(nowSec-float64(s.Anchor), total)

	lastIdx := len(s.Items) - 1
	var acc float64
	for i := range lastIdx {
		it := s.Items[i]
		if elapsed < acc+it.Duration {
			d := max(time.Duration((acc+it.Duration-elapsed)*float64(time.Second)), time.Nanosecond)
			return i, it, elapsed - acc, now.Add(d), true
		}
		acc += it.Duration
	}

	it := s.Items[lastIdx]
	d := max(time.Duration((acc+it.Duration-elapsed)*float64(time.Second)), time.Nanosecond)
	return lastIdx, it, elapsed - acc, now.Add(d), true
}

// earliestRemovedAir returns the soonest start, in old's looping timeline, of a file present in
// old but absent from present, and whether any such file exists. The result lies in [now,
// now+total): it is when the running transcode would next open (and fail on) the deleted file,
// used to decide whether a removal must be adopted before the daily swap. present is the set of
// files still on disk (from a cheap scan), so this needs no probe.
func earliestRemovedAir(old *Schedule, present map[string]struct{}, now time.Time) (time.Time, bool) {
	total := old.total()
	if total <= 0 {
		return time.Time{}, false
	}

	elapsed := mathMod(float64(now.UnixNano())/float64(time.Second)-float64(old.Anchor), total)

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
