package channelgen

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"
)

func contentItems(names ...string) []Item {
	items := make([]Item, len(names))
	for i, n := range names {
		items[i] = Item{File: n + ".mkv", Duration: 60}
	}
	return items
}

func fillerClip(id int, duration float64) Item {
	return Item{File: "ad" + strconv.Itoa(id) + ".mkv", Duration: duration, IsFiller: true}
}

func itemNames(items []Item) []string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = itemName(it)
	}
	return names
}

func TestInjectFillerSpacingAndFill(t *testing.T) {
	content := contentItems("a", "b", "c", "d", "e")
	pool := []Item{fillerClip(1, 10), fillerClip(2, 10), fillerClip(3, 10)}

	// every 2 content items, fill up to 25s -> two 10s clips per break (third would hit 30 > 25).
	out, _ := injectFiller(content, pool, 0, 2, 0, 25*time.Second)
	got := itemNames(out)

	want := []string{"a", "b", "ad1", "ad2", "c", "d", "ad3", "ad1", "e"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFillerZeroMaxDurationSingleClipNoTrailingBreak(t *testing.T) {
	content := contentItems("a", "b", "c", "d")
	pool := []Item{fillerClip(1, 5), fillerClip(2, 5)}

	// every 2 items, zero maxDuration: a single-clip break after b, and none after the last d.
	got, _ := injectFiller(content, pool, 0, 2, 0, 0)

	want := []string{"a", "b", "ad1", "c", "d"}
	if strings.Join(itemNames(got), ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", itemNames(got), want)
	}
}

func TestInjectFillerEmptyPoolNoop(t *testing.T) {
	content := contentItems("a", "b", "c")
	got, _ := injectFiller(content, nil, 0, 2, 0, time.Minute)
	if len(got) != 3 {
		t.Fatalf("empty pool must pass content through unchanged, got %d", len(got))
	}
}

func TestInjectFillerRotatesAcrossBuilds(t *testing.T) {
	content := contentItems("a", "b", "c") // one break (after b) of one clip; c is last
	pool := []Item{fillerClip(1, 5), fillerClip(2, 5), fillerClip(3, 5), fillerClip(4, 5)}

	// Feed each build's returned offset into the next; the single break must walk the whole
	// pool over four builds, then wrap.
	start := 0
	var aired []string
	for range 5 {
		out, next := injectFiller(content, pool, start, 2, 0, 0)
		aired = append(aired, itemNames(out)[2]) // a, b, [filler], c
		start = next
	}

	want := []string{"ad1", "ad2", "ad3", "ad4", "ad1"}
	if strings.Join(aired, ",") != strings.Join(want, ",") {
		t.Fatalf("expected rotation %v, got %v", want, aired)
	}
}

func TestInjectFillerTimeMode(t *testing.T) {
	// Uneven durations spread breaks naturally: with a 30s interval, a(40)>=30 breaks after a,
	// then b(10)+c(40)>=30 breaks after c, and d (last) gets none.
	content := []Item{
		{File: "a.mkv", Duration: 40},
		{File: "b.mkv", Duration: 10},
		{File: "c.mkv", Duration: 40},
		{File: "d.mkv", Duration: 10},
	}
	pool := []Item{fillerClip(1, 5)}

	out, _ := injectFiller(content, pool, 0, 0, 30*time.Second, 0)
	got := itemNames(out)

	want := []string{"a", "ad1", "b", "c", "ad1", "d"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFillerDefaultsToHourlyInterval(t *testing.T) {
	// Neither cadence set: falls back to a 1h content-playtime interval.
	content := []Item{
		{File: "a.mkv", Duration: 2400}, // 40m
		{File: "b.mkv", Duration: 2400}, // 40m -> cumulative 80m >= 60m, break here
		{File: "c.mkv", Duration: 600},  // 10m
	}
	pool := []Item{fillerClip(1, 5)}

	out, _ := injectFiller(content, pool, 0, 0, 0, 0)
	got := itemNames(out)

	want := []string{"a", "b", "ad1", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildScheduleInjectsFiller(t *testing.T) {
	dir := t.TempDir()
	fillerDir := t.TempDir()
	for _, f := range []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv"} {
		writeFile(t, filepath.Join(dir, f))
	}
	writeFile(t, filepath.Join(fillerDir, "spot.mkv"))

	p := newFakeProber()
	now := time.Unix(1000, 0)
	filler := FillerConfig{Sources: []string{fillerDir}, EveryCount: 2, Order: OrderSequential}

	s, err := tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "sequential", testSeasonPatterns, testEpisodePatterns, filler, nil, now)
	if err != nil {
		t.Fatal(err)
	}

	// a, b, [filler], c, d  -> one break after b (none after the last item d).
	if len(s.Items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(s.Items))
	}
	if !s.Items[2].IsFiller {
		t.Fatalf("expected filler at index 2, got %+v", s.Items[2])
	}
	if s.Items[len(s.Items)-1].IsFiller {
		t.Fatal("expected no trailing filler break")
	}
}

func TestProgrammesCollapsesFillerBreak(t *testing.T) {
	dir := t.TempDir()
	fillerDir := t.TempDir()
	for _, f := range []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv"} {
		writeFile(t, filepath.Join(dir, f))
	}
	for _, f := range []string{"spot1.mkv", "spot2.mkv"} {
		writeFile(t, filepath.Join(fillerDir, f))
	}

	p := newFakeProber()
	for _, f := range []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv"} {
		p.durations[filepath.Join(dir, f)] = 3600
	}
	p.durations[filepath.Join(fillerDir, "spot1.mkv")] = 30
	p.durations[filepath.Join(fillerDir, "spot2.mkv")] = 30

	c, _ := newTestChannel(t, "c", []string{dir}, p)
	c.filler = FillerConfig{
		Sources:       []string{fillerDir},
		EveryCount:    2,
		MaxDuration:   90 * time.Second,
		Order:         OrderSequential,
		TitleTemplate: template.Must(template.New("t").Parse("Advertising")),
	}
	now := time.Unix(10000, 0)
	warmUp(t, c, now)

	progs, err := c.Programmes(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}

	// A break fills to 90s (three 30s spots, the pool cycling) and must surface as a single
	// "Advertising" programme spanning the whole break, not one programme per spot.
	var breaks []Programme
	for _, pr := range progs {
		if pr.Title == "Advertising" {
			breaks = append(breaks, pr)
		}
	}
	if len(breaks) == 0 {
		t.Fatal("expected an Advertising programme")
	}
	for _, br := range breaks {
		if d := br.Stop.Sub(br.Start); d != 90*time.Second {
			t.Errorf("expected collapsed 90s filler break, got %s", d)
		}
		if br.Season != 0 || br.Episode != 0 {
			t.Errorf("filler programme must not carry season/episode, got %d/%d", br.Season, br.Episode)
		}
	}
}

func TestRebuildReusesUnchangedFiller(t *testing.T) {
	dir := t.TempDir()
	fillerDir := t.TempDir()
	for _, f := range []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv"} {
		writeFile(t, filepath.Join(dir, f))
	}
	writeFile(t, filepath.Join(fillerDir, "spot.mkv"))

	p := newFakeProber()
	now := time.Unix(1000, 0)
	filler := FillerConfig{Sources: []string{fillerDir}, EveryCount: 2, Order: OrderSequential}
	build := func(old *Schedule) *Schedule {
		s, err := tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "sequential", testSeasonPatterns, testEpisodePatterns, filler, old, now)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	first := build(nil)
	// Same files and config: the fingerprint matches, so the schedule is reused unchanged.
	second := build(first)

	if len(first.Items) != len(second.Items) {
		t.Fatalf("item count changed: %d -> %d", len(first.Items), len(second.Items))
	}
	for i := range first.Items {
		if first.Items[i].File != second.Items[i].File || first.Items[i].IsFiller != second.Items[i].IsFiller {
			t.Fatalf("layout changed at %d: %q(filler=%v) -> %q(filler=%v)",
				i, first.Items[i].File, first.Items[i].IsFiller, second.Items[i].File, second.Items[i].IsFiller)
		}
	}
}

func TestFillerShuffleSeededIndependentlyOfContent(t *testing.T) {
	dir := t.TempDir()
	fillerDir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ep1.mkv"))
	writeFile(t, filepath.Join(dir, "ep2.mkv"))
	// Enough filler clips that a fixed-zero seed (no shuffle) would leave them in scan order.
	for _, f := range []string{"ad1.mkv", "ad2.mkv", "ad3.mkv", "ad4.mkv", "ad5.mkv"} {
		writeFile(t, filepath.Join(fillerDir, f))
	}

	p := newFakeProber()
	now := time.Unix(1000, 0)
	// Content is sequential (seed stays 0); filler is shuffled and must still get a real seed.
	filler := FillerConfig{Sources: []string{fillerDir}, EveryCount: 2, Order: OrderShuffle}
	s, err := tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "sequential", testSeasonPatterns, testEpisodePatterns, filler, nil, now)
	if err != nil {
		t.Fatal(err)
	}

	if s.FillerSeed == 0 {
		t.Fatal("filler shuffle must use a non-zero seed independent of content order")
	}

	// Rebuild (new file) must keep the same FillerSeed so the pool order — and the FillerStart
	// cursor — stays stable.
	writeFile(t, filepath.Join(dir, "ep3.mkv"))
	s2, err := tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "sequential", testSeasonPatterns, testEpisodePatterns, filler, s, now)
	if err != nil {
		t.Fatal(err)
	}
	if s2.FillerSeed != s.FillerSeed {
		t.Errorf("FillerSeed must be sticky across rebuilds: %d -> %d", s.FillerSeed, s2.FillerSeed)
	}
}

func TestBuildRotatesSurplusFillerAcrossRebuilds(t *testing.T) {
	dir := t.TempDir()
	fillerDir := t.TempDir()
	// 3 content files, break after every 2 -> one break (after ep2) of one clip; ep3 is last.
	for _, f := range []string{"ep1.mkv", "ep2.mkv", "ep3.mkv"} {
		writeFile(t, filepath.Join(dir, f))
	}
	// Surplus pool: 3 clips, but only one break of one clip per loop.
	for _, f := range []string{"ad1.mkv", "ad2.mkv", "ad3.mkv"} {
		writeFile(t, filepath.Join(fillerDir, f))
	}

	p := newFakeProber()
	now := time.Unix(1000, 0)
	filler := FillerConfig{Sources: []string{fillerDir}, EveryCount: 2, Order: OrderSequential}
	build := func(old *Schedule) (*Schedule, error) {
		return tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "sequential", testSeasonPatterns, testEpisodePatterns, filler, old, now)
	}

	airedFiller := func(s *Schedule) string {
		for _, it := range s.Items {
			if it.IsFiller {
				return itemName(it)
			}
		}
		return ""
	}

	first, err := build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if airedFiller(first) != "ad1" || first.FillerStart != 1 {
		t.Fatalf("first build: aired %q, FillerStart %d; want ad1, 1", airedFiller(first), first.FillerStart)
	}

	// A no-op rebuild (same fingerprint) returns old untouched: offset must not advance.
	same, _ := build(first)
	if same.FillerStart != first.FillerStart {
		t.Errorf("unchanged rebuild advanced FillerStart: %d -> %d", first.FillerStart, same.FillerStart)
	}

	// A genuine rebuild (new content file) resumes from FillerStart, airing the next clip.
	writeFile(t, filepath.Join(dir, "ep4.mkv"))
	second, err := build(first)
	if err != nil {
		t.Fatal(err)
	}
	if airedFiller(second) != "ad2" || second.FillerStart != 2 {
		t.Fatalf("second build: aired %q, FillerStart %d; want ad2, 2", airedFiller(second), second.FillerStart)
	}
}

func TestRebuildReinterleavesFillerAfterReorder(t *testing.T) {
	dir := t.TempDir()
	fillerDir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	writeFile(t, a)
	writeFile(t, b)
	writeFile(t, filepath.Join(fillerDir, "spot.mkv"))

	p := newFakeProber()
	// Episodes live only in the container tag; flipping the pattern flips content order.
	p.results[a] = probeResult{Duration: 60, EpisodeTag: "Volume 2"}
	p.results[b] = probeResult{Duration: 60, EpisodeTag: "Volume 1"}

	now := time.Unix(1000, 0)
	filler := FillerConfig{Sources: []string{fillerDir}, EveryCount: 1, Order: OrderSequential}
	build := func(eps []*regexp.Regexp, old *Schedule) *Schedule {
		s, err := tBuild(context.Background(), p, "c", []string{dir}, testExtensions, "sequential", testSeasonPatterns, eps, filler, old, now)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	// No match: content stays in filename order a,b -> a, [filler], b.
	none := []*regexp.Regexp{regexp.MustCompile(`^$`)}
	first := build(none, nil)
	if got := itemNames(first.Items); strings.Join(got, ",") != "a,spot,b" {
		t.Fatalf("expected a,spot,b got %v", got)
	}

	// Pattern now orders by Volume -> b,a, and the filler break must follow the new order:
	// b, [filler], a. The rebuild reuses the probe cache, so content is not re-probed.
	volume := []*regexp.Regexp{regexp.MustCompile(`(?i)volume[ ._-]?(\d+)`)}
	second := build(volume, first)
	if got := itemNames(second.Items); strings.Join(got, ",") != "b,spot,a" {
		t.Fatalf("expected re-interleaved b,spot,a got %v", got)
	}
	if p.calls[a] != 1 || p.calls[b] != 1 {
		t.Errorf("rebuild must not re-probe content, got a=%d b=%d", p.calls[a], p.calls[b])
	}
}

func TestFingerprintChangesWithFiller(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mkv"))
	files, _ := scanSources([]string{dir}, testExtensions)

	fillerDir := t.TempDir()
	writeFile(t, filepath.Join(fillerDir, "spot.mkv"))
	fillerScanned, _ := scanSources([]string{fillerDir}, testExtensions)

	base := fingerprint(files, nil, "sequential", nil, nil, FillerConfig{})
	withFiller := fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 2, Order: OrderSequential})
	if base == withFiller {
		t.Error("fingerprint should change when filler files are added")
	}

	cadenceA := fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 2})
	cadenceB := fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 3})
	if cadenceA == cadenceB {
		t.Error("fingerprint should change when filler cadence changes")
	}

	if fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 2}) ==
		fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 2, MaxDuration: time.Minute}) {
		t.Error("fingerprint should change when filler max_duration changes")
	}

	if fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 2, Order: OrderSequential}) ==
		fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 2, Order: OrderShuffle}) {
		t.Error("fingerprint should change when filler order changes")
	}

	if fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{Every: 30 * time.Minute}) ==
		fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{Every: time.Hour}) {
		t.Error("fingerprint should change when filler interval changes")
	}

	if fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{EveryCount: 3}) ==
		fingerprint(files, fillerScanned, "sequential", nil, nil, FillerConfig{Every: time.Hour}) {
		t.Error("fingerprint should change when switching count<->time mode")
	}
}
