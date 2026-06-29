package channelgen

import (
	"context"
	"majmun/internal/hashid"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
)

var testExtensions = []string{"mkv", "mp4", "avi"}

// testSeasonPatterns / testEpisodePatterns mirror the production defaults for tests.
var testSeasonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(?:season|сезон|s)[ ._-]*\d{1,4}$`),
	regexp.MustCompile(`^\d{1,4}$`),
}

var testEpisodePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)s(\d{1,4})[ ._-]?e(\d{1,4})`),
	regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,4})\b`),
	regexp.MustCompile(`(?i)\bseason[ ._-]?(\d{1,4})\b.*?\bep(?:isode)?[ ._-]?(\d{1,4})\b`),
	regexp.MustCompile(`(?i)\bep(?:isode)?[ ._-]?(\d{1,4})\b`),
	regexp.MustCompile(`(?i)\be[ ._-]?(\d{1,4})\b`),
	regexp.MustCompile(`^\s*(\d{1,4})\.`),
}

type fakeProber struct {
	durations    map[string]float64
	titles       map[string]string
	descriptions map[string]string
	categories   map[string]string
	results      map[string]probeResult
	calls        map[string]int
}

func newFakeProber() *fakeProber {
	return &fakeProber{
		durations:    map[string]float64{},
		titles:       map[string]string{},
		descriptions: map[string]string{},
		categories:   map[string]string{},
		results:      map[string]probeResult{},
		calls:        map[string]int{},
	}
}

func (f *fakeProber) Probe(_ context.Context, file string) (probeResult, error) {
	f.calls[file]++
	if r, ok := f.results[file]; ok {
		return r, nil
	}
	dur := 60.0
	if d, ok := f.durations[file]; ok {
		dur = d
	}
	return probeResult{
		Duration:    dur,
		Title:       f.titles[file],
		Description: f.descriptions[file],
		Category:    f.categories[file],
	}, nil
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
}

var defaultTitleTmpl = template.Must(template.New("t").Funcs(sprig.FuncMap()).Parse("{{ .Probe.Title | default .File.Name }}"))

func newTestChannel(t *testing.T, id string, sources []string, p prober) (*Channel, string) {
	t.Helper()
	stateDir := t.TempDir()
	c := &Channel{id: id, sources: sources, extensions: testExtensions, epgDuration: 24 * time.Hour, stateDir: stateDir, prober: p, titleTmpl: defaultTitleTmpl}
	return c, stateDir
}

// tBuild wraps the cheap scan + probe build phases for tests, mirroring the pre-split
// buildSchedule signature so existing call sites keep working.
func tBuild(ctx context.Context, p prober, id string, sources, extensions []string, order Order, seasonPatterns, episodePatterns []*regexp.Regexp, filler FillerConfig, old *Schedule, now time.Time) (*Schedule, error) {
	scan, err := scanContent(sources, extensions, order, seasonPatterns, episodePatterns, filler)
	if err != nil {
		return nil, err
	}
	if old != nil && old.Fingerprint == scan.fingerprint {
		return old, nil
	}
	return buildSchedule(ctx, p, id, scan, sources, order, seasonPatterns, episodePatterns, filler, old, now), nil
}

func buildTestSchedule(p prober, dir string, order Order, old *Schedule, now time.Time) (*Schedule, error) {
	return tBuild(context.Background(), p, "c", []string{dir}, testExtensions, order, testSeasonPatterns, testEpisodePatterns, FillerConfig{}, old, now)
}

func itemName(it Item) string {
	base := filepath.Base(it.File)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// warmUp triggers a build and blocks until it has completed, so synchronous assertions
// can run against an async channel.
func warmUp(t *testing.T, c *Channel, now time.Time) {
	t.Helper()
	c.current(now)
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		done := !c.building.Load() && c.loaded
		c.mu.Unlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("build did not complete in time")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestScanSourcesFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "b.mkv"))
	writeFile(t, filepath.Join(dir, "a.mp4"))
	writeFile(t, filepath.Join(dir, "skip.txt"))
	writeFile(t, filepath.Join(dir, "sub", "c.avi"))

	files, err := scanSources([]string{dir}, testExtensions)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	if files[0].path != filepath.Join(dir, "a.mp4") {
		t.Errorf("expected sorted order, got %s first", files[0].path)
	}
}

func TestScanSourcesSkipsMissingSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	gone := filepath.Join(t.TempDir(), "gone.mkv")

	files, err := scanSources([]string{dir, gone}, testExtensions)
	if err != nil {
		t.Fatalf("a missing source must be skipped, not error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file from the surviving source, got %d", len(files))
	}
}

func TestFingerprintChangesWithFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	f1, _ := scanSources([]string{dir}, testExtensions)

	writeFile(t, filepath.Join(dir, "b.mkv"))
	f2, _ := scanSources([]string{dir}, testExtensions)

	if fingerprint(f1, nil, "sequential", nil, nil, FillerConfig{}) == fingerprint(f2, nil, "sequential", nil, nil, FillerConfig{}) {
		t.Error("fingerprint should change when files change")
	}
}

func TestFingerprintChangesWithOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	files, _ := scanSources([]string{dir}, testExtensions)

	if fingerprint(files, nil, "sequential", nil, nil, FillerConfig{}) == fingerprint(files, nil, "shuffle", nil, nil, FillerConfig{}) {
		t.Error("fingerprint should change when order changes")
	}
}

func TestFingerprintChangesWithPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	files, _ := scanSources([]string{dir}, testExtensions)

	p1 := []*regexp.Regexp{regexp.MustCompile(`^s(\d+)$`)}
	p2 := []*regexp.Regexp{regexp.MustCompile(`^season (\d+)$`)}
	if fingerprint(files, nil, "sequential", p1, nil, FillerConfig{}) == fingerprint(files, nil, "sequential", p2, nil, FillerConfig{}) {
		t.Error("fingerprint should change when season_patterns change")
	}
	if fingerprint(files, nil, "sequential", nil, p1, FillerConfig{}) == fingerprint(files, nil, "sequential", nil, p2, FillerConfig{}) {
		t.Error("fingerprint should change when episode_patterns change")
	}
}

func TestBuildScheduleSequential(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ep02.mkv"))
	writeFile(t, filepath.Join(dir, "ep01.mkv"))

	p := newFakeProber()
	p.durations[filepath.Join(dir, "ep01.mkv")] = 100
	p.durations[filepath.Join(dir, "ep02.mkv")] = 200

	now := time.Unix(1000, 0)

	s, err := buildTestSchedule(p, dir, "sequential", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(s.Items))
	}
	if itemName(s.Items[0]) != "ep01" || itemName(s.Items[1]) != "ep02" {
		t.Errorf("expected sorted order, got %s,%s", itemName(s.Items[0]), itemName(s.Items[1]))
	}
	if s.Anchor != now.Unix() {
		t.Errorf("expected anchor %d, got %d", now.Unix(), s.Anchor)
	}
}

func TestRebuildOnPatternChange(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	writeFile(t, a)
	writeFile(t, b)

	p := newFakeProber()
	// Episode lives only in the container tag; the rebuild re-derives it from the cached
	// EpisodeTag without re-probing.
	p.results[a] = probeResult{Duration: 60, EpisodeTag: "Volume 2"}
	p.results[b] = probeResult{Duration: 60, EpisodeTag: "Volume 1"}

	now := time.Unix(1000, 0)
	build := func(eps []*regexp.Regexp, old *Schedule) *Schedule {
		s, err := tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "sequential", testSeasonPatterns, eps, FillerConfig{}, old, now)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	none := []*regexp.Regexp{regexp.MustCompile(`^$`)}
	first := build(none, nil)
	if first.Items[0].Episode != 0 || first.Items[1].Episode != 0 {
		t.Fatalf("expected episode 0 before pattern match, got %d,%d", first.Items[0].Episode, first.Items[1].Episode)
	}
	if itemName(first.Items[0]) != "a" || itemName(first.Items[1]) != "b" {
		t.Fatalf("expected filename order a,b, got %s,%s", itemName(first.Items[0]), itemName(first.Items[1]))
	}

	volume := []*regexp.Regexp{regexp.MustCompile(`(?i)volume[ ._-]?(\d+)`)}
	second := build(volume, first)
	if itemName(second.Items[0]) != "b" || itemName(second.Items[1]) != "a" {
		t.Fatalf("expected re-derived order b,a, got %s,%s", itemName(second.Items[0]), itemName(second.Items[1]))
	}
	if second.Items[0].Episode != 1 || second.Items[1].Episode != 2 {
		t.Fatalf("expected episodes 1,2 after rebuild, got %d,%d", second.Items[0].Episode, second.Items[1].Episode)
	}
	if second.Anchor != first.Anchor || second.Seed != first.Seed {
		t.Errorf("rebuild must preserve anchor/seed: anchor %d->%d, seed %d->%d", first.Anchor, second.Anchor, first.Seed, second.Seed)
	}
	if p.calls[a] != 1 || p.calls[b] != 1 {
		t.Errorf("expected no re-probe, got calls a=%d b=%d", p.calls[a], p.calls[b])
	}
}

func TestRebuildPreservesShuffleOrder(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv", "e.mkv"} {
		writeFile(t, filepath.Join(dir, f))
	}
	p := newFakeProber()
	now := time.Unix(1000, 0)

	build := func(eps []*regexp.Regexp, old *Schedule) *Schedule {
		s, err := tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "shuffle", testSeasonPatterns, eps, FillerConfig{}, old, now)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	first := build(testEpisodePatterns, nil)
	second := build([]*regexp.Regexp{regexp.MustCompile(`(?i)volume[ ._-]?(\d+)`)}, first)

	if len(first.Items) != len(second.Items) {
		t.Fatalf("item count changed: %d -> %d", len(first.Items), len(second.Items))
	}
	for i := range first.Items {
		if first.Items[i].File != second.Items[i].File {
			t.Fatalf("shuffle order changed at %d: %s -> %s", i, itemName(first.Items[i]), itemName(second.Items[i]))
		}
	}
}

func TestBuildScheduleNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	// No episode info in tags or filenames; lexicographic order would be
	// part1, part10, part2 — natural order must win.
	for _, f := range []string{"part10.mkv", "part1.mkv", "part2.mkv"} {
		writeFile(t, filepath.Join(dir, f))
	}

	s, err := buildTestSchedule(newFakeProber(), dir, "sequential", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{itemName(s.Items[0]), itemName(s.Items[1]), itemName(s.Items[2])}
	want := []string{"part1", "part2", "part10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected natural order %v, got %v", want, got)
		}
	}
}

func TestBuildScheduleMixedTaggedUntagged(t *testing.T) {
	dir := t.TempDir()
	// Untagged files (season/episode 0/0) sort before tagged ones in the same
	// directory; the comparator must stay transitive across the mix.
	writeFile(t, filepath.Join(dir, "intro.mkv"))   // untagged
	writeFile(t, filepath.Join(dir, "zfinale.mkv")) // S02E01 via tags
	writeFile(t, filepath.Join(dir, "apilot.mkv"))  // S01E01 via tags

	p := newFakeProber()
	p.results[filepath.Join(dir, "zfinale.mkv")] = probeResult{Duration: 60, Season: 2, Episode: 1}
	p.results[filepath.Join(dir, "apilot.mkv")] = probeResult{Duration: 60, Season: 1, Episode: 1}

	s, err := buildTestSchedule(p, dir, "sequential", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{itemName(s.Items[0]), itemName(s.Items[1]), itemName(s.Items[2])}
	want := []string{"intro", "apilot", "zfinale"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected untagged-first order %v, got %v", want, got)
		}
	}
}

func TestBuildScheduleEpisodeOrder(t *testing.T) {
	dir := t.TempDir()
	// Filenames whose lexicographic AND natural order both contradict episode order:
	// the episode info comes from container tags.
	writeFile(t, filepath.Join(dir, "aaa.mkv")) // S02E01
	writeFile(t, filepath.Join(dir, "bbb.mkv")) // S01E02
	writeFile(t, filepath.Join(dir, "ccc.mkv")) // S01E01

	p := newFakeProber()
	p.results[filepath.Join(dir, "aaa.mkv")] = probeResult{Duration: 60, Season: 2, Episode: 1}
	p.results[filepath.Join(dir, "bbb.mkv")] = probeResult{Duration: 60, Season: 1, Episode: 2}
	p.results[filepath.Join(dir, "ccc.mkv")] = probeResult{Duration: 60, Season: 1, Episode: 1}

	s, err := buildTestSchedule(p, dir, "sequential", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{itemName(s.Items[0]), itemName(s.Items[1]), itemName(s.Items[2])}
	want := []string{"ccc", "bbb", "aaa"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected episode order %v, got %v", want, got)
		}
	}
}

func TestBuildScheduleEpisodeFromFilename(t *testing.T) {
	dir := t.TempDir()
	// No tags at all; SxxEyy must be picked up from the filename. Lexicographic order
	// (S01E10 < S01E2) would be wrong.
	writeFile(t, filepath.Join(dir, "Show S01E10.mkv"))
	writeFile(t, filepath.Join(dir, "Show S01E2.mkv"))
	writeFile(t, filepath.Join(dir, "Show S02E1.mkv"))

	s, err := buildTestSchedule(newFakeProber(), dir, "sequential", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{itemName(s.Items[0]), itemName(s.Items[1]), itemName(s.Items[2])}
	want := []string{"Show S01E2", "Show S01E10", "Show S02E1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected filename episode order %v, got %v", want, got)
		}
	}
	if s.Items[0].Season != 1 || s.Items[0].Episode != 2 {
		t.Errorf("expected S01E02 parsed from filename, got S%02dE%02d", s.Items[0].Season, s.Items[0].Episode)
	}
}

func TestBuildScheduleSeasonFromFolder(t *testing.T) {
	dir := t.TempDir()
	// Filename carries the episode but no season; the season comes from the folder.
	writeFile(t, filepath.Join(dir, "Show", "Season 3", "05. Episode.mkv"))

	s, err := buildTestSchedule(newFakeProber(), dir, "sequential", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if s.Items[0].Season != 3 || s.Items[0].Episode != 5 {
		t.Errorf("expected S03E05 (season from folder, episode from filename), got S%02dE%02d",
			s.Items[0].Season, s.Items[0].Episode)
	}
}

func TestBuildScheduleGroupsByDirectory(t *testing.T) {
	dir := t.TempDir()
	// Episode metadata must not interleave files across different source directories.
	writeFile(t, filepath.Join(dir, "show-b", "E01.mkv"))
	writeFile(t, filepath.Join(dir, "show-a", "E02.mkv"))

	s, err := buildTestSchedule(newFakeProber(), dir, "sequential", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if itemName(s.Items[0]) != "E02" || itemName(s.Items[1]) != "E01" {
		t.Errorf("expected directory grouping (show-a first), got %s,%s", itemName(s.Items[0]), itemName(s.Items[1]))
	}
}

func TestBuildScheduleUsesContainerMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ep01.mkv"))
	writeFile(t, filepath.Join(dir, "ep02.mkv"))

	p := newFakeProber()
	p.results[filepath.Join(dir, "ep01.mkv")] = probeResult{
		Duration:    60,
		Title:       "The Beautiful Episode",
		Description: "A short synopsis.",
		Category:    "Animation",
		Date:        "20210608",
		Season:      1,
		Episode:     3,
	}
	// ep02 has no tags -> raw title stays empty, episode parsed from filename ("ep02" -> 2),
	// so it sorts before ep01's tagged episode 3.

	s, err := buildTestSchedule(p, dir, "sequential", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	it := s.Items[1]
	if it.Title != "The Beautiful Episode" || it.Description != "A short synopsis." || it.Category != "Animation" {
		t.Errorf("unexpected title/desc/category: %+v", it)
	}
	if it.Date != "20210608" || it.Season != 1 || it.Episode != 3 {
		t.Errorf("unexpected date/season/episode: %+v", it)
	}
	if s.Items[0].Title != "" {
		t.Errorf("expected raw empty title for untagged file, got %q", s.Items[0].Title)
	}
	if s.Items[0].Description != "" || s.Items[0].Category != "" || s.Items[0].Date != "" {
		t.Errorf("expected empty metadata for untagged file, got %+v", s.Items[0])
	}
	if s.Items[0].Episode != 2 {
		t.Errorf("expected episode 2 from filename, got %d", s.Items[0].Episode)
	}
}

func TestParseDate(t *testing.T) {
	cases := map[string]string{
		"2021-06-08":          "20210608",
		"2021":                "20210101",
		"20210608":            "20210608",
		"2021-06-08 12:00:00": "20210608",
		"":                    "",
		"not a date":          "",
		"June 2021":           "",
	}
	for in, want := range cases {
		if got := parseDate(in); got != want {
			t.Errorf("parseDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildScheduleReusesCacheNoReprobe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))

	p := newFakeProber()
	now := time.Unix(1000, 0)

	s1, err := buildTestSchedule(p, dir, "sequential", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := buildTestSchedule(p, dir, "sequential", s1, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if s2.Fingerprint != s1.Fingerprint || s2.Anchor != s1.Anchor || s2.Seed != s1.Seed {
		t.Error("expected unchanged schedule to be reused")
	}
	if len(s2.Items) != len(s1.Items) || s2.Items[0].File != s1.Items[0].File {
		t.Error("expected items to be preserved")
	}
	if p.calls[filepath.Join(dir, "a.mkv")] != 1 {
		t.Errorf("expected exactly 1 probe, got %d", p.calls[filepath.Join(dir, "a.mkv")])
	}
}

func TestBuildSchedulePreservesAnchorAndSeed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))

	p := newFakeProber()
	now := time.Unix(1000, 0)

	s1, _ := buildTestSchedule(p, dir, "shuffle", nil, now)

	writeFile(t, filepath.Join(dir, "b.mkv"))
	s2, _ := buildTestSchedule(p, dir, "shuffle", s1, now.Add(time.Hour))

	if s2.Anchor != s1.Anchor {
		t.Errorf("anchor should be preserved: %d != %d", s2.Anchor, s1.Anchor)
	}
	if s2.Seed != s1.Seed {
		t.Errorf("seed should be preserved: %d != %d", s2.Seed, s1.Seed)
	}
}

func TestSeriesKey(t *testing.T) {
	// Library-style source: the source is a parent of show folders. A file dropped directly
	// into the source groups under the source's own name (not as a series of one).
	libCases := map[string]string{
		"/mnt/series/Show/S01E01.mkv":         "Show",
		"/mnt/series/Show/Season 1/ep01.mkv":  "Show",
		"/mnt/series/Show/Сезон - 2/ep01.mkv": "Show",
		"/mnt/series/Show/01/ep01.mkv":        "Show",
		"/mnt/series/loose.mkv":               "series",
	}
	for file, want := range libCases {
		if got := seriesKey(file, []string{"/mnt/series"}, testSeasonPatterns); got != want {
			t.Errorf("seriesKey(%q, lib) = %q, want %q", file, got, want)
		}
	}

	// Show-style source: the source is itself a show. Seasons beneath it, or episodes placed
	// directly inside it, both group under the show's name.
	showCases := map[string]string{
		"/mnt/series/Show/Сезон 2/ep01.mkv":  "Show",
		"/mnt/series/Show/Season 1/ep01.mkv": "Show",
		"/mnt/series/Show/S03/ep01.mkv":      "Show",
		"/mnt/series/Show/ep01.mkv":          "Show",
	}
	for file, want := range showCases {
		if got := seriesKey(file, []string{"/mnt/series/Show"}, testSeasonPatterns); got != want {
			t.Errorf("seriesKey(%q, show) = %q, want %q", file, got, want)
		}
	}

	// A file under no configured source falls back to its filename.
	if got := seriesKey("/elsewhere/x.mkv", []string{"/mnt/series"}, testSeasonPatterns); got != "x.mkv" {
		t.Errorf("seriesKey(no source) = %q, want %q", got, "x.mkv")
	}

	// A show folder may itself be named like a season; the leading name stays the show.
	seasonNamedCases := map[string]string{
		"/mnt/series/Season 25/Season 01/ep.mkv":     "Season 25",
		"/mnt/series/2020/Season 01/ep.mkv":          "2020",
		"/mnt/series/xxx/Season 01/Season 02/ep.mkv": "xxx",
	}
	for file, want := range seasonNamedCases {
		if got := seriesKey(file, []string{"/mnt/series"}, testSeasonPatterns); got != want {
			t.Errorf("seriesKey(%q, season-named) = %q, want %q", file, got, want)
		}
	}
}

func TestSpreadOrder(t *testing.T) {
	dir := t.TempDir()
	p := newFakeProber()
	// Show A: 2 eps (pos .25 .75); Show B: 4 eps (pos .125 .375 .625 .875).
	for _, f := range []string{
		"A/S01E01.mkv", "A/S01E02.mkv",
		"B/S01E01.mkv", "B/S01E02.mkv", "B/S01E03.mkv", "B/S01E04.mkv",
	} {
		path := filepath.Join(dir, f)
		writeFile(t, path)
		p.durations[path] = 100
	}

	s, err := buildTestSchedule(p, dir, "spread", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range s.Items {
		got = append(got, filepath.Base(filepath.Dir(it.File))+"/"+filepath.Base(it.File))
	}
	want := []string{
		"B/S01E01.mkv", "A/S01E01.mkv", "B/S01E02.mkv",
		"B/S01E03.mkv", "A/S01E02.mkv", "B/S01E04.mkv",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %v, want %v", i, got, want)
		}
	}
}

func TestInterleaveOrder(t *testing.T) {
	dir := t.TempDir()
	p := newFakeProber()
	// Show A: 2 eps; Show B: 3 eps. Expect A1,B1,A2,B2,B3 (A drops out, B continues).
	for _, f := range []string{
		"A/S01E01.mkv", "A/S01E02.mkv",
		"B/S01E01.mkv", "B/S01E02.mkv", "B/S01E03.mkv",
	} {
		path := filepath.Join(dir, f)
		writeFile(t, path)
		p.durations[path] = 100
	}

	s, err := buildTestSchedule(p, dir, "interleave", nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range s.Items {
		got = append(got, filepath.Base(filepath.Dir(it.File))+"/"+filepath.Base(it.File))
	}
	want := []string{
		"A/S01E01.mkv", "B/S01E01.mkv",
		"A/S01E02.mkv", "B/S01E02.mkv",
		"B/S01E03.mkv",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %v, want %v", i, got, want)
		}
	}
}

// Each show is its own source with season subfolders. Interleave must rotate across shows,
// not across season folders (the regression: "Сезон 2/3/..." treated as separate shows).
func TestInterleaveGroupsByShowAcrossSeasonSources(t *testing.T) {
	root := t.TempDir()
	p := newFakeProber()
	files := []string{
		"Show A/Сезон 1/01.mkv", "Show A/Сезон 2/01.mkv",
		"Show B/Сезон 1/01.mkv", "Show B/Сезон 1/02.mkv",
	}
	for _, f := range files {
		path := filepath.Join(root, f)
		writeFile(t, path)
		p.durations[path] = 100
	}
	sources := []string{filepath.Join(root, "Show A"), filepath.Join(root, "Show B")}

	s, err := tBuild(context.Background(), p, "c", sources, testExtensions, "interleave", testSeasonPatterns, testEpisodePatterns, FillerConfig{}, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range s.Items {
		got = append(got, filepath.Base(filepath.Dir(filepath.Dir(it.File)))+"/"+filepath.Base(filepath.Dir(it.File))+"/"+filepath.Base(it.File))
	}
	want := []string{
		"Show A/Сезон 1/01.mkv", "Show B/Сезон 1/01.mkv",
		"Show A/Сезон 2/01.mkv", "Show B/Сезон 1/02.mkv",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %v, want %v", i, got, want)
		}
	}
}

func TestInterleaveGroupsByShowTag(t *testing.T) {
	root := t.TempDir()
	p := newFakeProber()
	files := map[string]string{
		"dirX/a1.mkv": "Show A",
		"dirY/a2.mkv": "Show A",
		"dirZ/b1.mkv": "Show B",
	}
	for f, show := range files {
		path := filepath.Join(root, f)
		writeFile(t, path)
		p.results[path] = probeResult{Duration: 100, Show: show}
	}

	s, err := tBuild(context.Background(), p, "c", []string{root}, testExtensions, "interleave", testSeasonPatterns, testEpisodePatterns, FillerConfig{}, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range s.Items {
		got = append(got, it.Show+"/"+filepath.Base(it.File))
	}
	want := []string{"Show A/a1.mkv", "Show B/b1.mkv", "Show A/a2.mkv"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %v, want %v", i, got, want)
		}
	}
}

func TestInterleaveSeasonPatternOverride(t *testing.T) {
	root := t.TempDir()
	p := newFakeProber()
	files := []string{
		"Show A/Vol 1/01.mkv", "Show A/Vol 2/01.mkv",
		"Show B/Vol 1/01.mkv",
	}
	for _, f := range files {
		path := filepath.Join(root, f)
		writeFile(t, path)
		p.durations[path] = 100
	}
	sources := []string{filepath.Join(root, "Show A"), filepath.Join(root, "Show B")}
	seasonPatterns := []*regexp.Regexp{regexp.MustCompile(`(?i)^vol[ ._-]*\d+$`)}

	s, err := tBuild(context.Background(), p, "c", sources, testExtensions, "interleave", seasonPatterns, testEpisodePatterns, FillerConfig{}, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range s.Items {
		got = append(got, filepath.Base(filepath.Dir(filepath.Dir(it.File)))+"/"+filepath.Base(it.File))
	}
	want := []string{"Show A/01.mkv", "Show B/01.mkv", "Show A/01.mkv"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %v, want %v", i, got, want)
		}
	}
}

func TestLocateModulo(t *testing.T) {
	s := &Schedule{
		Anchor: 1000,
		Items: []Item{
			{File: "a", Title: "a", Duration: 100},
			{File: "b", Title: "b", Duration: 200},
		},
	}
	// total = 300. now = anchor + 350 -> elapsed 50 into second cycle -> item a, offset 50.
	index, item, offset, boundary, ok := locate(s, time.Unix(1350, 0))
	if !ok {
		t.Fatal("expected ok")
	}
	if index != 0 || item.File != "a" {
		t.Errorf("expected item a at index 0, got %d/%s", index, item.File)
	}
	if offset != 50 {
		t.Errorf("expected offset 50, got %f", offset)
	}
	if boundary.Unix() != 1400 {
		t.Errorf("expected boundary 1400, got %d", boundary.Unix())
	}
}

func TestLocateSecondItem(t *testing.T) {
	s := &Schedule{
		Anchor: 0,
		Items: []Item{
			{File: "a", Duration: 100},
			{File: "b", Duration: 200},
		},
	}
	// elapsed 150 -> item b at index 1, offset 50.
	index, item, offset, _, ok := locate(s, time.Unix(150, 0))
	if !ok || index != 1 || item.File != "b" || offset != 50 {
		t.Errorf("expected b/50 at index 1, got %d/%s/%f ok=%v", index, item.File, offset, ok)
	}
}

func TestLocateSubSecondBoundary(t *testing.T) {
	s := &Schedule{
		Anchor: 0,
		Items: []Item{
			{File: "a", Duration: 10},
			{File: "b", Duration: 10},
		},
	}
	// 0.06s before the a->b boundary at 10.0s: stay in a with a future boundary.
	now := time.Unix(9, int64(0.94*float64(time.Second)))
	index, item, offset, boundary, ok := locate(s, now)
	if !ok || index != 0 || item.File != "a" {
		t.Fatalf("expected item a, got %d/%s ok=%v", index, item.File, ok)
	}
	if d := offset - 9.94; d < -1e-6 || d > 1e-6 {
		t.Errorf("expected offset ~9.94, got %f", offset)
	}
	if !boundary.After(now) {
		t.Errorf("boundary %v must be after now %v so the resolver can cross it", boundary, now)
	}

	// Just past the boundary: advance to b at offset ~0, not linger in a's tail.
	now2 := time.Unix(10, int64(0.02*float64(time.Second)))
	index2, item2, offset2, _, ok2 := locate(s, now2)
	if !ok2 || index2 != 1 || item2.File != "b" {
		t.Fatalf("expected item b after the boundary, got %d/%s ok=%v", index2, item2.File, ok2)
	}
	if offset2 < 0 || offset2 > 0.1 {
		t.Errorf("expected b offset ~0.02, got %f", offset2)
	}
}

// TestLocateInvariants fuzzes locate, asserting the offset lies inside the resolved slot and the
// boundary is strictly in the future.
func TestLocateInvariants(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 100000; i++ {
		n := 1 + r.Intn(8)
		items := make([]Item, n)
		for j := range items {
			items[j].File = string(rune('a' + j))
			items[j].Duration = 0.05 + r.Float64()*600 // realistic clip seconds
		}
		s := &Schedule{Anchor: r.Int63n(2e9), Items: items}
		now := time.Unix(1_600_000_000+r.Int63n(2e8), r.Int63n(int64(time.Second)))

		index, item, offset, boundary, ok := locate(s, now)
		if !ok {
			t.Fatalf("ok=false for non-empty schedule (i=%d)", i)
		}
		if offset < 0 || offset >= item.Duration {
			t.Fatalf("offset %v outside [0,%v) index=%d (i=%d)", offset, item.Duration, index, i)
		}
		if !boundary.After(now) {
			t.Fatalf("boundary %v not after now %v index=%d (i=%d)", boundary, now, index, i)
		}
	}
}

// TestLocateBoundaryAtSeam drives now to each slot/cycle end, where the remaining-time delta is
// sub-nanosecond and truncates to zero; the boundary must still be strictly after now.
func TestLocateBoundaryAtSeam(t *testing.T) {
	s := &Schedule{
		Anchor: 1000,
		Items: []Item{
			{File: "a", Duration: 0.1},
			{File: "b", Duration: 0.2},
			{File: "c", Duration: 0.3},
		},
	}
	total := s.total()
	base := time.Unix(s.Anchor, 0)

	var acc float64
	for k := 0; k <= len(s.Items); k++ {
		if k < len(s.Items) {
			acc += s.Items[k].Duration
		} else {
			acc = total
		}
		// At the slot end and a few nanoseconds either side, the delta to the boundary is tiny.
		for _, dn := range []int64{-2, -1, 0, 1, 2} {
			now := base.Add(time.Duration(acc*float64(time.Second)) + time.Duration(dn))
			_, item, offset, boundary, ok := locate(s, now)
			if !ok {
				t.Fatalf("k=%d dn=%d: ok=false", k, dn)
			}
			if offset < 0 || offset >= item.Duration {
				t.Errorf("k=%d dn=%d: offset %v outside [0,%v)", k, dn, offset, item.Duration)
			}
			if !boundary.After(now) {
				t.Errorf("k=%d dn=%d: boundary %v not after now %v", k, dn, boundary, now)
			}
		}
	}
}

func TestLocateEmpty(t *testing.T) {
	s := &Schedule{Anchor: 0}
	if _, _, _, _, ok := locate(s, time.Unix(10, 0)); ok {
		t.Error("expected not ok for empty schedule")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	stateDir := t.TempDir()
	s := &Schedule{
		Channel:     "c",
		Fingerprint: "sha256:abc",
		Anchor:      1000,
		Items:       []Item{{File: "/a.mkv", Title: "a", Duration: 60}},
	}
	if err := saveSchedule(stateDir, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSchedule(stateDir, "c")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Fingerprint != "sha256:abc" || len(loaded.Items) != 1 {
		t.Errorf("roundtrip mismatch: %+v", loaded)
	}
}

func TestLoadScheduleMissing(t *testing.T) {
	loaded, err := loadSchedule(t.TempDir(), "nope")
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Error("expected nil for missing schedule")
	}
}

func activeFingerprint(c *Channel) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.schedule == nil {
		return ""
	}
	return c.schedule.Fingerprint
}

func TestResolveCurrentSkipsMissing(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.mkv")
	writeFile(t, good)

	p := newFakeProber()
	c, _ := newTestChannel(t, "c", []string{dir}, p)

	// Inject a schedule whose first item is gone so the current slot must skip forward.
	// loaded + lastBuilt + high refresh keep current() from rebuilding it out from under us.
	gone := filepath.Join(dir, "gone.mkv")
	now := time.Unix(1030, 0) // 30s into the (missing) first slot
	c.refresh = time.Hour
	c.loaded = true
	c.lastBuilt = now
	c.schedule = &Schedule{
		Channel: c.id,
		Anchor:  1000,
		Items: []Item{
			{File: gone, Duration: 100},
			{File: good, Duration: 100},
		},
	}

	it, ok := c.ResolveCurrent(now)
	if !ok {
		t.Fatal("resolve failed")
	}
	if it.File != good {
		t.Errorf("expected surviving file, got %q", it.File)
	}
	if it.Offset != 0 {
		t.Errorf("expected offset reset to 0, got %f", it.Offset)
	}
}

func TestResolveCurrentOffset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))

	p := newFakeProber()
	p.durations[filepath.Join(dir, "a.mkv")] = 100
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	now := time.Unix(1000, 0)
	warmUp(t, c, now)

	resolveAt := now.Add(30 * time.Second)
	it, ok := c.ResolveCurrent(resolveAt)
	if !ok {
		t.Fatal("resolve failed")
	}
	if it.Offset != 30 {
		t.Errorf("expected offset 30, got %f", it.Offset)
	}
	// Single 100s file: its slot ends 70s after a resolve at offset 30.
	wantBoundary := resolveAt.Add(70 * time.Second)
	if !it.NextBoundary.Equal(wantBoundary) {
		t.Errorf("expected NextBoundary %v, got %v", wantBoundary, it.NextBoundary)
	}
}

func TestResolveCurrentCatchUp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))

	p := newFakeProber()
	p.durations[filepath.Join(dir, "a.mkv")] = 100
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	now := time.Unix(1000, 0)
	warmUp(t, c, now)

	// 40s in the past wraps onto the 100s loop at offset 60.
	it, ok := c.ResolveCurrent(now.Add(-40 * time.Second))
	if !ok {
		t.Fatal("resolve failed")
	}
	if it.Offset != 60 {
		t.Errorf("expected offset 60, got %f", it.Offset)
	}
}

func TestClampStart(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))

	p := newFakeProber()
	p.durations[filepath.Join(dir, "a.mkv")] = 100
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	now := time.Unix(10000, 0)
	warmUp(t, c, now)

	past := now.Add(-time.Hour)
	if got := c.ClampStart(past, now); !got.Equal(past) {
		t.Errorf("past: expected %v, got %v", past, got)
	}
	if got := c.ClampStart(now.Add(time.Hour), now); !got.Equal(now) {
		t.Errorf("future: expected %v, got %v", now, got)
	}
}

func TestCatchupWindow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	p := newFakeProber()
	p.durations[filepath.Join(dir, "a.mkv")] = 3600 // 1h loop
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.epgDuration = 7 * 24 * time.Hour
	now := time.Unix(1000, 0)
	warmUp(t, c, now)

	// loop (1h) < epg (7d) -> loop wins
	if got := c.CatchupWindow(now); got != time.Hour {
		t.Errorf("loop cap: expected 1h, got %v", got)
	}
	c.epgDuration = 30 * time.Minute
	if got := c.CatchupWindow(now); got != 30*time.Minute {
		t.Errorf("epg cap: expected 30m, got %v", got)
	}
}

func TestResolveCurrentEmptyChannel(t *testing.T) {
	dir := t.TempDir() // no media files
	p := newFakeProber()
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	now := time.Unix(1000, 0)
	warmUp(t, c, now)

	_, ok := c.ResolveCurrent(now)
	if ok {
		t.Error("expected ok=false for empty channel")
	}
}

func TestProgrammesGrid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	writeFile(t, filepath.Join(dir, "b.mkv"))

	p := newFakeProber()
	p.durations[filepath.Join(dir, "a.mkv")] = 3600
	p.durations[filepath.Join(dir, "b.mkv")] = 3600

	c, _ := newTestChannel(t, "c", []string{dir}, p)
	now := time.Unix(10000, 0)
	warmUp(t, c, now)

	progs, err := c.Programmes(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(progs) == 0 {
		t.Fatal("expected programmes")
	}
	for i := 1; i < len(progs); i++ {
		if !progs[i].Start.Equal(progs[i-1].Stop) {
			t.Errorf("gap between programme %d and %d", i-1, i)
		}
	}
	if progs[0].Title != "a" && progs[0].Title != "b" {
		t.Errorf("unexpected title %s", progs[0].Title)
	}
}

func TestProgrammesTitleTemplatePrefixesShow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Дальнобойщики", "S01E05.mkv"))

	p := newFakeProber()
	p.results[filepath.Join(dir, "Дальнобойщики", "S01E05.mkv")] = probeResult{Duration: 3600, Title: "В рейс"}

	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.titleTmpl = template.Must(template.New("t").Funcs(sprig.FuncMap()).
		Parse(`{{ .File.Rel | splitList "/" | first }} — {{ .Probe.Title | default .File.Name }}`))

	now := time.Unix(10000, 0)
	warmUp(t, c, now)

	progs, err := c.Programmes(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(progs) == 0 {
		t.Fatal("expected programmes")
	}
	if progs[0].Title != "Дальнобойщики — В рейс" {
		t.Errorf("unexpected title %q", progs[0].Title)
	}
}

func TestProgrammesTitleTemplateRelNoExt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Show", "Season 1", "05. Episode.mkv"))

	p := newFakeProber()
	p.results[filepath.Join(dir, "Show", "Season 1", "05. Episode.mkv")] = probeResult{Duration: 3600}

	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.titleTmpl = template.Must(template.New("t").Funcs(sprig.FuncMap()).Parse(`{{ .File.RelNoExt }}`))

	now := time.Unix(10000, 0)
	warmUp(t, c, now)

	progs, err := c.Programmes(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(progs) == 0 {
		t.Fatal("expected programmes")
	}
	if progs[0].Title != "Show/Season 1/05. Episode" {
		t.Errorf("unexpected title %q", progs[0].Title)
	}
}

func TestProgrammesTitleTemplateSourceBase(t *testing.T) {
	dir := t.TempDir()
	show := filepath.Join(dir, "My Show")
	writeFile(t, filepath.Join(show, "Season 1", "20. Episode.mkv"))

	p := newFakeProber()
	p.results[filepath.Join(show, "Season 1", "20. Episode.mkv")] = probeResult{Duration: 3600}

	c, _ := newTestChannel(t, "c", []string{show}, p)
	c.titleTmpl = template.Must(template.New("t").Funcs(sprig.FuncMap()).
		Parse(`{{ .File.SourceBase }}/{{ .File.RelNoExt }}`))

	now := time.Unix(10000, 0)
	warmUp(t, c, now)

	progs, err := c.Programmes(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(progs) == 0 {
		t.Fatal("expected programmes")
	}
	if progs[0].Title != "My Show/Season 1/20. Episode" {
		t.Errorf("unexpected title %q", progs[0].Title)
	}
}

func TestProgrammesTitleTemplateErrorFallsBackToRaw(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))

	p := newFakeProber()
	p.results[filepath.Join(dir, "a.mkv")] = probeResult{Duration: 3600, Title: "Raw Title"}

	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.titleTmpl = template.Must(template.New("t").Funcs(sprig.FuncMap()).Parse(`{{ .Probe.Missing }}`))

	now := time.Unix(10000, 0)
	warmUp(t, c, now)

	progs, err := c.Programmes(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(progs) == 0 {
		t.Fatal("expected programmes")
	}
	if progs[0].Title != "Raw Title" {
		t.Errorf("expected raw fallback, got %q", progs[0].Title)
	}
}

// TestNewChannelIDContract pins the id derivation shared with app.Channel.ID(): the server
// resolves a channel by the app-side id while the generator loads its schedule by this one,
// so the two must agree or schedule lookup breaks silently.
func TestNewChannelIDContract(t *testing.T) {
	c := NewChannel(Config{Playlist: "local", Name: "cartoons", Extensions: testExtensions, Order: "sequential", EPGDuration: 24 * time.Hour, SwapHour: 4, StateDir: "state"})
	if c.id != hashid.New("local", "cartoons") {
		t.Errorf("expected id to hash the playlist and channel names, got %s", c.id)
	}
}

func TestBuildPersistsScheduleFlat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	p := newFakeProber()
	// Hostile characters in names never reach the filesystem: the id is a hashid.
	c := NewChannel(Config{Playlist: "local", Name: "name with? hostile/chars ", Sources: []string{dir}, Extensions: testExtensions, Order: "sequential", EPGDuration: 24 * time.Hour, SwapHour: 4, StateDir: t.TempDir()})
	c.prober = p
	stateDir := c.stateDir

	warmUp(t, c, time.Unix(1000, 0))

	path := schedulePath(stateDir, c.id)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("schedule not persisted: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(stateDir, "channels") {
		t.Errorf("expected flat layout, got %q", path)
	}
	if base := filepath.Base(path); strings.ContainsAny(base, "/? ") {
		t.Errorf("expected sanitized filename, got %q", base)
	}

	// Load round-trips by id and the stored identity matches the filename.
	s, err := loadSchedule(stateDir, c.id)
	if err != nil || s == nil {
		t.Fatalf("loadSchedule: %v, %v", s, err)
	}
	if s.Channel != c.id {
		t.Errorf("unexpected channel identity %q", s.Channel)
	}
}

func TestPruneSchedules(t *testing.T) {
	stateDir := t.TempDir()
	save := func(id string) {
		if err := saveSchedule(stateDir, &Schedule{Channel: id}); err != nil {
			t.Fatal(err)
		}
	}
	save("aaaa000000000001")
	save("aaaa000000000002")
	save("aaaa000000000003")

	if err := PruneSchedules(stateDir, []string{"aaaa000000000001"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(schedulePath(stateDir, "aaaa000000000001")); err != nil {
		t.Errorf("kept schedule was removed: %v", err)
	}
	if _, err := os.Stat(schedulePath(stateDir, "aaaa000000000002")); !os.IsNotExist(err) {
		t.Error("orphaned schedule should be pruned")
	}
	if _, err := os.Stat(schedulePath(stateDir, "aaaa000000000003")); !os.IsNotExist(err) {
		t.Error("orphaned schedule should be pruned")
	}
}

func TestPruneSchedulesCleansLegacyLayout(t *testing.T) {
	stateDir := t.TempDir()
	// Pre-hashing layout: name-based files nested in per-playlist subdirs.
	legacy := filepath.Join(stateDir, "channels", "playlist")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "series.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := PruneSchedules(stateDir, []string{"aaaa000000000001"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(legacy, "series.json")); !os.IsNotExist(err) {
		t.Error("legacy name-based schedule should be pruned")
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("emptied legacy subdir should be removed")
	}
}

func TestPruneSchedulesMissingDir(t *testing.T) {
	if err := PruneSchedules(t.TempDir(), []string{"aaaa000000000001"}); err != nil {
		t.Errorf("missing channels dir should not error: %v", err)
	}
}

func TestResolveDoesNotBlockOnFirstBuild(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	p := newFakeProber()
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	now := time.Unix(1000, 0)

	// First call must return immediately without a built schedule.
	if _, ok := c.ResolveCurrent(now); ok {
		t.Error("expected ok=false while first build runs")
	}

	warmUp(t, c, now)

	if _, ok := c.ResolveCurrent(now); !ok {
		t.Error("expected ok=true after build completes")
	}
}

func TestLoadedScheduleServedWithoutRebuild(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	p := newFakeProber()

	c, stateDir := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = 0
	warmUp(t, c, time.Unix(1000, 0))
	probesAfterFirst := p.calls[filepath.Join(dir, "a.mkv")]

	// Fresh channel over the same state dir loads the persisted schedule and serves it
	// immediately, without probing again.
	c2 := &Channel{id: "c", sources: []string{dir}, extensions: testExtensions, stateDir: stateDir, prober: p}
	if c2.current(time.Unix(2000, 0)) == nil {
		t.Fatal("expected loaded schedule to be served immediately")
	}
	if p.calls[filepath.Join(dir, "a.mkv")] != probesAfterFirst {
		t.Error("loaded schedule should not trigger a reprobe")
	}
}

func TestRefreshIntervalGatesRebuild(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	p := newFakeProber()
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour

	now := time.Unix(1000, 0)
	warmUp(t, c, now)
	first := c.lastBuilt

	// Within the interval: no rebuild.
	c.current(now.Add(time.Minute))
	if !c.building.Load() && !c.lastBuilt.Equal(first) {
		t.Error("rebuild should be gated within refresh interval")
	}

	// Past the interval: triggers a rebuild.
	c.current(now.Add(2 * time.Hour))
	deadline := time.Now().Add(2 * time.Second)
	for c.building.Load() && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if c.lastBuilt.Equal(first) {
		t.Error("expected rebuild after refresh interval elapsed")
	}
}

// hookProber runs a callback during Probe, simulating a file change that arrives while a
// build is in progress.
type hookProber struct {
	onProbe func()
}

func (h *hookProber) Probe(_ context.Context, _ string) (probeResult, error) {
	if h.onProbe != nil {
		h.onProbe()
	}
	return probeResult{Duration: 60}, nil
}

func TestDirtySignalDuringBuildSurvives(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))

	c, _ := newTestChannel(t, "c", []string{dir}, nil)
	c.prober = &hookProber{onProbe: func() { c.markDirty(time.Unix(1000, 0)) }}

	// Hold the single-flight guard as maybeBuild would, so the mid-build markDirty
	// cannot spawn a competing build (its CAS fails) — matching real execution.
	c.building.Store(true)
	c.build(context.Background(), time.Unix(1000, 0))
	c.building.Store(false)

	c.mu.Lock()
	dirty := c.dirty
	c.mu.Unlock()
	if !dirty {
		t.Error("dirty signal arriving mid-build must survive, so the next access rebuilds")
	}
}

func TestScheduleChangeDeferredUntilSwapTime(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	p.durations[a] = 60
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule
	if first == nil {
		t.Fatal("expected first schedule adopted immediately")
	}

	// A daytime addition is detected but neither probed nor adopted: the live schedule keeps
	// playing until the swap window.
	b := filepath.Join(dir, "b.mkv")
	p.durations[b] = 60
	writeFile(t, b)
	warmUp(t, c, t0.Add(2*time.Hour))

	if activeFingerprint(c) != first.Fingerprint {
		t.Error("live schedule must keep playing until the swap window")
	}
	if p.calls[b] != 0 {
		t.Errorf("daytime change must not probe the new file, got %d probes", p.calls[b])
	}

	// In the swap window the rebuild adopts the change (and only now probes the new file).
	want := time.Date(2024, 1, 2, 4, 0, 0, 0, time.Local)
	warmUp(t, c, want.Add(time.Minute))
	if activeFingerprint(c) == first.Fingerprint {
		t.Error("expected the change to be adopted in the swap window")
	}
	if p.calls[b] != 1 {
		t.Errorf("expected the new file probed once on adoption, got %d", p.calls[b])
	}
}

// TestDeferredChangeAdoptedAtSwapWindow guards that a change deferred shortly before the swap
// window is still adopted when the window arrives, even though less than a refresh interval has
// elapsed since it was last scanned. A purely refresh-gated rebuild would skip the window scan
// and leave the change stuck until the next interval.
func TestDeferredChangeAdoptedAtSwapWindow(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	p.durations[a] = 60
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule.Fingerprint

	// New file detected at 03:30, half an hour before the 04:00 window: deferred.
	b := filepath.Join(dir, "b.mkv")
	p.durations[b] = 60
	writeFile(t, b)
	warmUp(t, c, time.Date(2024, 1, 2, 3, 30, 0, 0, time.Local))
	if activeFingerprint(c) != first {
		t.Fatal("precondition: change must be deferred before the window")
	}

	// At 04:01, only 31m after the deferred scan (< refresh), the channel must still rebuild and
	// adopt at the window.
	c.mu.Lock()
	need := c.needsBuildLocked(time.Date(2024, 1, 2, 4, 1, 0, 0, time.Local))
	c.mu.Unlock()
	if !need {
		t.Error("a deferred change must arm a rebuild at the swap window, not wait for the refresh interval")
	}

	warmUp(t, c, time.Date(2024, 1, 2, 4, 1, 0, 0, time.Local))
	if activeFingerprint(c) == first {
		t.Error("deferred change must be adopted at the swap window")
	}
}

// TestUrgentRemovalDoesNotCutHeldProgramme guards that after the swap time, while the programme
// spanning the window is still airing (the swap is held), a removed file airing only after that
// held programme finishes does not trigger an early adoption mid-programme.
func TestUrgentRemovalDoesNotCutHeldProgramme(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "a_keep.mkv")
	gone := filepath.Join(dir, "z_gone.mkv")
	writeFile(t, keep)
	writeFile(t, gone)
	p := newFakeProber()
	// keep airs from the 10:00 anchor for 20h -> the programme spanning the next 04:00 window
	// ends at 06:00. gone airs 06:00..07:00, i.e. only after that held boundary.
	p.durations[keep] = 20 * 3600
	p.durations[gone] = 1 * 3600
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule.Fingerprint

	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	// 04:10: past the swap time but inside the held programme (keep, ending 06:00). gone airs at
	// 06:00, not before the held boundary, so it must NOT cut in early.
	c.building.Store(true)
	c.build(context.Background(), time.Date(2024, 1, 2, 4, 10, 0, 0, time.Local))
	c.building.Store(false)

	if activeFingerprint(c) != first {
		t.Error("a removal airing only after the held programme boundary must not adopt mid-programme")
	}
}

// TestDeferredChangeReArmsAtProgrammeBoundary guards that when the swap window opens but the
// airing programme runs past it, the deferred change re-arms a rebuild at that programme's
// boundary, not at the next day's window. A long refresh interval would otherwise mask the bug.
func TestDeferredChangeReArmsAtProgrammeBoundary(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	// a airs from the 10:00 anchor for 20h, so the programme spanning the next 04:00 window ends
	// at 06:00.
	p.durations[a] = 20 * 3600
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = 12 * time.Hour // long, so it cannot mask the re-arm timing
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule.Fingerprint

	b := filepath.Join(dir, "b.mkv")
	p.durations[b] = 60
	writeFile(t, b)

	// 04:10: window open but held by the programme ending at 06:00 -> deferred.
	c.building.Store(true)
	c.build(context.Background(), time.Date(2024, 1, 2, 4, 10, 0, 0, time.Local))
	c.building.Store(false)
	if activeFingerprint(c) != first {
		t.Fatal("precondition: held at 04:10")
	}

	// 06:01: the held programme has finished -> a rebuild must be armed and adopt now.
	c.mu.Lock()
	need := c.needsBuildLocked(time.Date(2024, 1, 2, 6, 1, 0, 0, time.Local))
	c.mu.Unlock()
	if !need {
		t.Error("a held change must re-arm at the programme boundary, not the next day's window")
	}
	warmUp(t, c, time.Date(2024, 1, 2, 6, 1, 0, 0, time.Local))
	if activeFingerprint(c) == first {
		t.Error("held change must be adopted once the programme finishes")
	}
}

// TestRemovedFileSwapTiming pins the urgent-removal decision: a removed file is adopted early
// only when its next airing falls at or before the swap boundary (the running transcode would
// otherwise reopen it). keepDur/goneDur place gone's slot relative to the next 04:00 (18h after
// the 10:00 anchor): before it, exactly on it, or after it.
func TestRemovedFileSwapTiming(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	cases := []struct {
		name             string
		keepDur, goneDur float64
		wantEarly        bool
	}{
		// 60s items loop every 2min, so gone airs again minutes after removal, before the swap.
		{"before swap", 60, 60, true},
		// keep airs 0..18h, so gone's slot starts exactly at 18h == the swap instant.
		{"at swap boundary", 18 * 3600, 3600, true},
		// 40h items: gone's slot starts at 40h, past tomorrow's 04:00 (18h away).
		{"after swap", 40 * 3600, 40 * 3600, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			keep := filepath.Join(dir, "a_keep.mkv")
			gone := filepath.Join(dir, "z_gone.mkv")
			writeFile(t, keep)
			writeFile(t, gone)
			p := newFakeProber()
			p.durations[keep] = tc.keepDur
			p.durations[gone] = tc.goneDur
			c, stateDir := newTestChannel(t, "c", []string{dir}, p)
			c.refresh = time.Hour
			c.swapHour, c.swapMin = 4, 0

			warmUp(t, c, t0)
			first := c.schedule

			if err := os.Remove(gone); err != nil {
				t.Fatal(err)
			}
			warmUp(t, c, t0.Add(2*time.Hour))

			c.mu.Lock()
			active := c.schedule
			c.mu.Unlock()

			if tc.wantEarly {
				if active == first {
					t.Error("removal airing at or before the swap must adopt the new schedule immediately")
				}
				// The early-adopted schedule must be persisted, so a restart keeps it.
				saved, err := loadSchedule(stateDir, "c")
				if err != nil || saved == nil {
					t.Fatalf("expected persisted schedule, got %v err %v", saved, err)
				}
				if saved.Fingerprint != active.Fingerprint {
					t.Error("persisted schedule must match the early-adopted one")
				}
			} else if active != first {
				t.Error("removal airing after the swap must defer, not switch early")
			}
		})
	}
}

func TestRemovalSupersedesPendingAddition(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	writeFile(t, a)
	writeFile(t, b)
	p := newFakeProber()
	p.durations[a] = 60
	p.durations[b] = 60
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)

	first := c.schedule

	// A daytime addition is held: the live schedule keeps playing.
	writeFile(t, filepath.Join(dir, "c.mkv"))
	warmUp(t, c, t0.Add(2*time.Hour))
	if activeFingerprint(c) != first.Fingerprint {
		t.Fatal("daytime addition should be held, not adopted")
	}

	// A removal that airs before the swap supersedes the held addition and adopts immediately.
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	warmUp(t, c, t0.Add(3*time.Hour))
	if activeFingerprint(c) == first.Fingerprint {
		t.Error("removal airing before swap must adopt the corrected schedule immediately")
	}
}

func TestEmptyRebuildKeepsPlayingSchedule(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule

	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	warmUp(t, c, t0.Add(2*time.Hour))

	c.mu.Lock()
	active := c.schedule
	c.mu.Unlock()
	if active != first {
		t.Error("emptied sources must not displace the playing schedule")
	}
}

// TestUnchangedEmptyScheduleSkipsProbe guards the cheap-scan short-circuit for empty schedules:
// when every file probes to no duration, the schedule is empty but the scan fingerprint is
// stable, so an unchanged refresh must not re-run the probe pass.
func TestUnchangedEmptyScheduleSkipsProbe(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	p.durations[a] = 0 // probes to zero duration: scan succeeds, schedule is empty
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour

	warmUp(t, c, time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local))
	probed := p.calls[a]

	c.building.Store(true)
	c.build(context.Background(), time.Date(2024, 1, 1, 12, 0, 0, 0, time.Local))
	c.building.Store(false)

	if p.calls[a] != probed {
		t.Errorf("unchanged empty schedule must not re-probe, calls grew %d -> %d", probed, p.calls[a])
	}
}

// TestEmptyRebuildAfterDeferredChangeClearsArm guards against a rebuild spin: when a change is
// deferred (arming a swap) and then all content is removed before the boundary, the empty rebuild
// that keeps the old schedule must also clear the armed swap, or needsBuildLocked would stay true
// and rebuild on every access.
func TestEmptyRebuildAfterDeferredChangeClearsArm(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	writeFile(t, a)
	writeFile(t, b)
	p := newFakeProber()
	p.durations[a] = 60
	p.durations[b] = 60
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)

	// Defer a change at 03:30, arming a swap.
	cc := filepath.Join(dir, "c.mkv")
	p.durations[cc] = 60
	writeFile(t, cc)
	warmUp(t, c, time.Date(2024, 1, 2, 3, 30, 0, 0, time.Local))
	c.mu.Lock()
	armed := !c.pendingSwapAt.IsZero()
	c.mu.Unlock()
	if !armed {
		t.Fatal("precondition: a swap must be armed after deferral")
	}

	// All content removed before the boundary; the build at the boundary yields an empty schedule
	// that must keep the old one and clear the arm.
	os.Remove(a)
	os.Remove(b)
	os.Remove(cc)
	c.building.Store(true)
	c.build(context.Background(), time.Date(2024, 1, 2, 4, 1, 0, 0, time.Local))
	c.building.Store(false)

	c.mu.Lock()
	stillArmed := !c.pendingSwapAt.IsZero()
	need := c.needsBuildLocked(time.Date(2024, 1, 2, 4, 2, 0, 0, time.Local))
	c.mu.Unlock()
	if stillArmed {
		t.Error("empty rebuild must clear the armed swap")
	}
	if need {
		t.Error("empty rebuild must not leave needsBuildLocked stuck true (rebuild spin)")
	}
}

// TestRestartBeforeSwapWindowHoldsChange covers a restart while a daytime change is held: the
// reloaded channel must keep playing the active schedule and not adopt the change until the
// swap window, even though it rebuilds at startup.
func TestRestartBeforeSwapWindowHoldsChange(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	p.durations[a] = 60
	c, stateDir := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	firstFp := c.schedule.Fingerprint

	// A daytime change appears, then the process "restarts" (fresh channel, same state dir).
	b := filepath.Join(dir, "b.mkv")
	p.durations[b] = 60
	writeFile(t, b)

	c2 := &Channel{id: "c", sources: []string{dir}, extensions: testExtensions, stateDir: stateDir, prober: p}
	c2.refresh = time.Hour
	c2.swapHour, c2.swapMin = 4, 0
	warmUp(t, c2, t0.Add(3*time.Hour)) // still 2024-01-01, before the next 04:00

	if activeFingerprint(c2) != firstFp {
		t.Error("restart before the swap window must keep the active schedule, not adopt the change")
	}
	if p.calls[b] != 0 {
		t.Errorf("restart before the swap window must not probe the held change, got %d", p.calls[b])
	}
}

// TestDeferredSwapSurvivesRestart covers persistence of the armed swap boundary: a change
// detected and deferred before a restart keeps its original swap time, so a restart before the
// boundary still adopts at that boundary instead of re-deferring to the next day.
func TestDeferredSwapSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	p.durations[a] = 60
	c, stateDir := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	firstFp := c.schedule.Fingerprint

	// A change is detected and deferred at 03:30, arming (and persisting) a swap at ~04:00.
	b := filepath.Join(dir, "b.mkv")
	p.durations[b] = 60
	writeFile(t, b)
	warmUp(t, c, time.Date(2024, 1, 2, 3, 30, 0, 0, time.Local))
	if activeFingerprint(c) != firstFp {
		t.Fatal("precondition: change deferred at 03:30")
	}

	// Restart: a fresh channel must restore the armed boundary from disk, then adopt at it.
	c2 := &Channel{id: "c", sources: []string{dir}, extensions: testExtensions, stateDir: stateDir, prober: p}
	c2.refresh = time.Hour
	c2.swapHour, c2.swapMin = 4, 0

	c2.mu.Lock()
	c2.ensureLoadedLocked()
	restoredArm := !c2.pendingSwapAt.IsZero()
	c2.mu.Unlock()
	if !restoredArm {
		t.Error("restart must restore the persisted pending swap boundary")
	}

	warmUp(t, c2, time.Date(2024, 1, 2, 4, 1, 0, 0, time.Local))
	if activeFingerprint(c2) == firstFp {
		t.Error("restart before the swap boundary must still adopt the deferred change at it")
	}
	if p.calls[b] != 1 {
		t.Errorf("expected the change probed once on adoption, got %d", p.calls[b])
	}
}

// TestChangeFirstSeenAfterWindowDefers covers the core lazy guarantee: a non-urgent change first
// detected after the swap window's programme has already finished is held to the next window, not
// adopted immediately just because the active schedule was built earlier.
func TestChangeFirstSeenAfterWindowDefers(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	p.durations[a] = 60 // short programmes, so the 04:00 window's programme ends right away
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule.Fingerprint

	// Jan 2 04:00 passes untouched; a new file appears and is first scanned at Jan 2 12:00, well
	// after that window. It must defer (not probe, not adopt), waiting for the next window.
	b := filepath.Join(dir, "b.mkv")
	p.durations[b] = 60
	writeFile(t, b)
	c.building.Store(true)
	c.build(context.Background(), time.Date(2024, 1, 2, 12, 0, 0, 0, time.Local))
	c.building.Store(false)

	if activeFingerprint(c) != first {
		t.Error("a change first seen after the window must defer, not adopt immediately")
	}
	if p.calls[b] != 0 {
		t.Errorf("a deferred change must not be probed, got %d", p.calls[b])
	}
}

// TestFillerChannelDefersDaytimeChange guards against filler clips being mistaken for removed
// files: their paths come from the filler sources, not the content scan, so the lazy swap's
// removed-file check must still see them as present and defer a daytime content change rather
// than forcing an urgent adoption (and reprobe) on every refresh.
func TestFillerChannelDefersDaytimeChange(t *testing.T) {
	dir := t.TempDir()
	fillerDir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	spot := filepath.Join(fillerDir, "spot.mkv")
	writeFile(t, a)
	writeFile(t, b)
	writeFile(t, spot)
	p := newFakeProber()
	p.durations[a] = 3600
	p.durations[b] = 3600
	p.durations[spot] = 30
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0
	c.filler = FillerConfig{
		Sources:       []string{fillerDir},
		EveryCount:    1,
		MaxDuration:   30 * time.Second,
		Order:         OrderSequential,
		TitleTemplate: template.Must(template.New("t").Parse("Advertising")),
	}

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule
	if first == nil {
		t.Fatal("expected first schedule")
	}
	// Sanity: filler must actually be interleaved, or the test wouldn't exercise the bug.
	hasFiller := false
	for _, it := range first.Items {
		if it.IsFiller {
			hasFiller = true
		}
	}
	if !hasFiller {
		t.Fatal("expected filler interleaved in the schedule")
	}

	// A daytime content addition must be deferred, not adopted, even though the schedule is
	// interleaved with filler clips whose paths are absent from the content scan.
	cc := filepath.Join(dir, "c.mkv")
	p.durations[cc] = 3600
	writeFile(t, cc)
	warmUp(t, c, t0.Add(2*time.Hour))

	if activeFingerprint(c) != first.Fingerprint {
		t.Error("filler clips must not be mistaken for removed files forcing an urgent swap")
	}
	if p.calls[cc] != 0 {
		t.Errorf("deferred daytime change must not be probed, got %d", p.calls[cc])
	}
}
