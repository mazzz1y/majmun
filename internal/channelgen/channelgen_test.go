package channelgen

import (
	"context"
	"majmun/internal/hashid"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testExtensions = []string{"mkv", "mp4", "avi"}

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

func newTestChannel(t *testing.T, id string, sources []string, p prober) (*Channel, string) {
	t.Helper()
	stateDir := t.TempDir()
	c := &Channel{id: id, sources: sources, extensions: testExtensions, epgDuration: 24 * time.Hour, stateDir: stateDir, prober: p}
	return c, stateDir
}

func buildTestSchedule(p prober, dir string, randomOrder bool, old *Schedule, now time.Time) (*Schedule, error) {
	return buildSchedule(context.Background(), p, "c", []string{dir}, testExtensions, randomOrder, old, now)
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

func TestFingerprintChangesWithFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	f1, _ := scanSources([]string{dir}, testExtensions)

	writeFile(t, filepath.Join(dir, "b.mkv"))
	f2, _ := scanSources([]string{dir}, testExtensions)

	if fingerprint(f1, false) == fingerprint(f2, false) {
		t.Error("fingerprint should change when files change")
	}
}

func TestFingerprintChangesWithRandomOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	files, _ := scanSources([]string{dir}, testExtensions)

	if fingerprint(files, false) == fingerprint(files, true) {
		t.Error("fingerprint should change when random_order changes")
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

	s, err := buildTestSchedule(p, dir, false, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(s.Items))
	}
	if s.Items[0].Title != "ep01" || s.Items[1].Title != "ep02" {
		t.Errorf("expected sorted titles, got %s,%s", s.Items[0].Title, s.Items[1].Title)
	}
	if s.Anchor != now.Unix() {
		t.Errorf("expected anchor %d, got %d", now.Unix(), s.Anchor)
	}
}

func TestBuildScheduleNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	// No episode info in tags or filenames; lexicographic order would be
	// part1, part10, part2 — natural order must win.
	for _, f := range []string{"part10.mkv", "part1.mkv", "part2.mkv"} {
		writeFile(t, filepath.Join(dir, f))
	}

	s, err := buildTestSchedule(newFakeProber(), dir, false, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{s.Items[0].Title, s.Items[1].Title, s.Items[2].Title}
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

	s, err := buildTestSchedule(p, dir, false, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{s.Items[0].Title, s.Items[1].Title, s.Items[2].Title}
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

	s, err := buildTestSchedule(p, dir, false, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{s.Items[0].Title, s.Items[1].Title, s.Items[2].Title}
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

	s, err := buildTestSchedule(newFakeProber(), dir, false, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{s.Items[0].Title, s.Items[1].Title, s.Items[2].Title}
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

func TestBuildScheduleGroupsByDirectory(t *testing.T) {
	dir := t.TempDir()
	// Episode metadata must not interleave files across different source directories.
	writeFile(t, filepath.Join(dir, "show-b", "E01.mkv"))
	writeFile(t, filepath.Join(dir, "show-a", "E02.mkv"))

	s, err := buildTestSchedule(newFakeProber(), dir, false, nil, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if s.Items[0].Title != "E02" || s.Items[1].Title != "E01" {
		t.Errorf("expected directory grouping (show-a first), got %s,%s", s.Items[0].Title, s.Items[1].Title)
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
	// ep02 has no tags -> filename title, episode parsed from filename ("ep02" -> 2),
	// so it sorts before ep01's tagged episode 3.

	s, err := buildTestSchedule(p, dir, false, nil, time.Unix(1000, 0))
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
	if s.Items[0].Title != "ep02" {
		t.Errorf("expected filename fallback, got %q", s.Items[0].Title)
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

	s1, err := buildTestSchedule(p, dir, false, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := buildTestSchedule(p, dir, false, s1, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if s2 != s1 {
		t.Error("expected unchanged schedule to be reused")
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

	s1, _ := buildTestSchedule(p, dir, true, nil, now)

	writeFile(t, filepath.Join(dir, "b.mkv"))
	s2, _ := buildTestSchedule(p, dir, true, s1, now.Add(time.Hour))

	if s2.Anchor != s1.Anchor {
		t.Errorf("anchor should be preserved: %d != %d", s2.Anchor, s1.Anchor)
	}
	if s2.Seed != s1.Seed {
		t.Errorf("seed should be preserved: %d != %d", s2.Seed, s1.Seed)
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

	it, ok := c.ResolveCurrent(now.Add(30 * time.Second))
	if !ok {
		t.Fatal("resolve failed")
	}
	if it.Offset != 30 {
		t.Errorf("expected offset 30, got %f", it.Offset)
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

// TestNewChannelIDContract pins the id derivation shared with app.Channel.ID(): the server
// resolves a channel by the app-side id while the generator loads its schedule by this one,
// so the two must agree or schedule lookup breaks silently.
func TestNewChannelIDContract(t *testing.T) {
	c := NewChannel("local", "cartoons", nil, testExtensions, false, 0, 24*time.Hour, 4, 0, "state")
	if c.id != hashid.New("local", "cartoons") {
		t.Errorf("expected id to hash the playlist and channel names, got %s", c.id)
	}
}

func TestBuildPersistsScheduleFlat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	p := newFakeProber()
	// Hostile characters in names never reach the filesystem: the id is a hashid.
	c := NewChannel("local", "name with? hostile/chars ", []string{dir}, testExtensions, false, 0, 24*time.Hour, 4, 0, t.TempDir())
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

	writeFile(t, filepath.Join(dir, "b.mkv"))
	warmUp(t, c, t0.Add(2*time.Hour))

	c.mu.Lock()
	pending, active, promoteAt := c.pending, c.schedule, c.promoteAt
	c.mu.Unlock()
	if pending == nil {
		t.Fatal("expected changed schedule to be deferred in pending")
	}
	if active != first {
		t.Error("live schedule must keep playing until the swap time")
	}
	want := time.Date(2024, 1, 2, 4, 0, 0, 0, time.Local)
	if !promoteAt.Equal(want) {
		t.Errorf("promoteAt = %v, want next 04:00 = %v", promoteAt, want)
	}

	c.ResolveCurrent(t0.Add(2 * time.Hour))
	if activeFingerprint(c) != first.Fingerprint {
		t.Error("schedule must not change before the swap time")
	}

	c.ResolveCurrent(want.Add(time.Second))
	if activeFingerprint(c) != first.Fingerprint {
		t.Error("swap must wait for the current item to finish, not cut mid-show")
	}
	c.mu.Lock()
	heldPending := c.pending
	c.mu.Unlock()
	if heldPending == nil {
		t.Error("pending must still be held until the item boundary")
	}

	if _, ok := c.ResolveCurrent(want.Add(60 * time.Second).Add(time.Second)); !ok {
		t.Fatal("resolve failed after item boundary")
	}
	if activeFingerprint(c) == first.Fingerprint {
		t.Error("expected pending schedule to be promoted after the item boundary")
	}
	c.mu.Lock()
	stillPending := c.pending
	c.mu.Unlock()
	if stillPending != nil {
		t.Error("pending should be cleared after promotion")
	}
}

func TestDeferredSwapTimeNotPostponedByRebuilds(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFile(t, a)
	p := newFakeProber()
	p.durations[a] = 60
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = 5 * time.Minute
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)

	writeFile(t, filepath.Join(dir, "b.mkv"))
	warmUp(t, c, t0.Add(10*time.Minute))

	c.mu.Lock()
	first := c.promoteAt
	pendingFp := c.pending.Fingerprint
	c.mu.Unlock()
	want := time.Date(2024, 1, 2, 4, 0, 0, 0, time.Local)
	if !first.Equal(want) {
		t.Fatalf("promoteAt = %v, want %v", first, want)
	}

	// Subsequent refresh rebuilds of the same pending change must not push the swap time out.
	for _, d := range []time.Duration{20 * time.Minute, 6 * time.Hour, 16 * time.Hour} {
		warmUp(t, c, t0.Add(d))
		c.mu.Lock()
		got, fp := c.promoteAt, c.pending.Fingerprint
		c.mu.Unlock()
		if !got.Equal(want) {
			t.Errorf("at +%v: promoteAt = %v, want unchanged %v", d, got, want)
		}
		if fp != pendingFp {
			t.Errorf("at +%v: pending fingerprint changed unexpectedly", d)
		}
	}
}

func TestRemovedFileAiringBeforeSwapSwapsEarly(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	writeFile(t, a)
	writeFile(t, b)
	p := newFakeProber()
	p.durations[a] = 60
	p.durations[b] = 60
	c, stateDir := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule

	// Total cycle is 120s, so any surviving file airs again within minutes — well before the
	// next 04:00. Removing one must adopt the corrected schedule immediately.
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	warmUp(t, c, t0.Add(2*time.Hour))

	c.mu.Lock()
	active, pending, promoteAt := c.schedule, c.pending, c.promoteAt
	c.mu.Unlock()
	if active == first {
		t.Error("removed file airing before swap must adopt the new schedule immediately")
	}
	if len(active.Items) != 1 {
		t.Errorf("expected 1 surviving item, got %d", len(active.Items))
	}
	if pending != nil || !promoteAt.IsZero() {
		t.Error("immediate adoption must clear any pending swap")
	}

	// The early-adopted schedule must be persisted, so a restart keeps it.
	saved, err := loadSchedule(stateDir, "c")
	if err != nil || saved == nil {
		t.Fatalf("expected persisted schedule, got %v err %v", saved, err)
	}
	if saved.Fingerprint != active.Fingerprint {
		t.Error("persisted schedule must match the early-adopted one")
	}
}

func TestRemovedFileAiringAfterSwapDefers(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "a_keep.mkv")
	gone := filepath.Join(dir, "z_gone.mkv")
	writeFile(t, keep)
	writeFile(t, gone)
	p := newFakeProber()
	// Long items so the playlist does not loop before the swap: the first item alone covers
	// far beyond the next 04:00, pushing z_gone's slot past the swap time.
	p.durations[keep] = 40 * 3600
	p.durations[gone] = 40 * 3600
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule

	// Sanity: a_keep airs from anchor, z_gone only after 40h — past tomorrow 04:00 (18h away).
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	warmUp(t, c, t0.Add(2*time.Hour))

	c.mu.Lock()
	active, pending := c.schedule, c.pending
	c.mu.Unlock()
	if active != first {
		t.Error("removal airing after the swap must defer, not switch early")
	}
	if pending == nil {
		t.Error("expected the corrected schedule to be deferred in pending")
	}
}

// TestRemovedFileAiringAtSwapBoundarySwapsEarly pins the boundary-vs-swapAt fix: a removal
// airing exactly at the swap time would be reached live before adoption, so it must swap early.
func TestRemovedFileAiringAtSwapBoundarySwapsEarly(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "a_keep.mkv")
	gone := filepath.Join(dir, "b_gone.mkv")
	writeFile(t, keep)
	writeFile(t, gone)
	p := newFakeProber()
	// Anchor is t0 = 10:00; the next 04:00 swap is 18h away. keep airs 0..18h, so gone's slot
	// starts exactly at 18h == the swap instant, making gone the programme straddling the
	// boundary (ends at 19h). gone's next occurrence therefore lands exactly on swapAt.
	p.durations[keep] = 18 * 3600
	p.durations[gone] = 3600
	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.refresh = time.Hour
	c.swapHour, c.swapMin = 4, 0

	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	warmUp(t, c, t0)
	first := c.schedule

	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	warmUp(t, c, t0.Add(2*time.Hour))

	c.mu.Lock()
	active, pending, promoteAt := c.schedule, c.pending, c.promoteAt
	c.mu.Unlock()
	if active == first {
		t.Error("removal airing at the swap boundary must adopt the new schedule immediately")
	}
	if pending != nil || !promoteAt.IsZero() {
		t.Error("immediate adoption must clear any pending swap")
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

	// An addition is deferred.
	writeFile(t, filepath.Join(dir, "c.mkv"))
	warmUp(t, c, t0.Add(2*time.Hour))
	c.mu.Lock()
	deferred := c.pending != nil
	c.mu.Unlock()
	if !deferred {
		t.Fatal("addition should have been deferred")
	}

	// A removal that airs before the swap must supersede the pending addition and adopt now.
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	warmUp(t, c, t0.Add(3*time.Hour))
	c.mu.Lock()
	pending, promoteAt := c.pending, c.promoteAt
	c.mu.Unlock()
	if pending != nil || !promoteAt.IsZero() {
		t.Error("removal airing before swap must supersede the pending addition and adopt now")
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
	active, pending := c.schedule, c.pending
	c.mu.Unlock()
	if active != first {
		t.Error("emptied sources must not displace the playing schedule")
	}
	if pending != nil {
		t.Error("an empty rebuild must not be queued as pending")
	}
}
