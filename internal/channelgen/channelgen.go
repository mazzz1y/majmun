package channelgen

import (
	"context"
	"errors"
	"majmun/internal/ctxutil"
	"majmun/internal/hashid"
	"majmun/internal/logging"
	"regexp"
	"sync"
	"sync/atomic"
	"text/template"
	"time"
)

type Order string

const (
	OrderSequential Order = "sequential"
	OrderShuffle    Order = "shuffle"
	OrderInterleave Order = "interleave"
	OrderSpread     Order = "spread"
)

type Config struct {
	Playlist        string
	Name            string
	Sources         []string
	Extensions      []string
	Order           Order
	SeasonPatterns  []*regexp.Regexp
	EpisodePatterns []*regexp.Regexp
	Refresh         time.Duration
	EPGDuration     time.Duration
	SwapHour        int
	SwapMin         int
	StateDir        string

	TitleTemplate       *template.Template
	DescriptionTemplate *template.Template
	CategoryTemplate    *template.Template

	Filler FillerConfig
}

type FillerConfig struct {
	Sources          []string
	EveryCount       int
	Every            time.Duration
	MaxDuration      time.Duration
	Order            Order
	TitleTemplate    *template.Template
	CategoryTemplate *template.Template
}

type Channel struct {
	// id is the channel's hashid, shared with app.Channel.ID(): schedule file name,
	// the schedule JSON's "channel" field, and the tvg-id all carry this same token.
	id              string
	playlist        string
	name            string
	sources         []string
	extensions      []string
	order           Order
	seasonPatterns  []*regexp.Regexp
	episodePatterns []*regexp.Regexp
	refresh         time.Duration
	epgDuration     time.Duration
	swapHour        int
	swapMin         int
	stateDir        string
	filler          FillerConfig
	prober          prober

	titleTmpl       *template.Template
	descriptionTmpl *template.Template
	categoryTmpl    *template.Template

	mu            sync.Mutex
	schedule      *Schedule
	loaded        bool
	dirty         bool
	lastBuilt     time.Time
	pendingSwapAt time.Time
	building      atomic.Bool

	// resumeMu is separate from mu: cursors are per-viewer session state read on the streaming
	// path, where blocking on a rebuild would stall an unrelated viewer.
	resumeMu sync.Mutex
	resume   map[string]resumeCursor
}

func NewChannel(cfg Config) *Channel {
	return &Channel{
		id:              hashid.New(cfg.Playlist, cfg.Name),
		playlist:        cfg.Playlist,
		name:            cfg.Name,
		sources:         cfg.Sources,
		extensions:      cfg.Extensions,
		order:           cfg.Order,
		seasonPatterns:  cfg.SeasonPatterns,
		episodePatterns: cfg.EpisodePatterns,
		refresh:         cfg.Refresh,
		epgDuration:     cfg.EPGDuration,
		swapHour:        cfg.SwapHour,
		swapMin:         cfg.SwapMin,
		stateDir:        cfg.StateDir,
		filler:          cfg.Filler,
		prober:          ffprobeProber{},
		titleTmpl:       cfg.TitleTemplate,
		descriptionTmpl: cfg.DescriptionTemplate,
		categoryTmpl:    cfg.CategoryTemplate,
	}
}

// current returns the servable schedule without blocking. It is nil while the first build
// runs (callers serve a placeholder). A background build is triggered when the channel is
// unbuilt, marked dirty by a deleted file, past its refresh interval, or due for a deferred swap.
func (c *Channel) current(now time.Time) *Schedule {
	c.mu.Lock()
	c.ensureLoadedLocked()
	s := c.schedule
	need := c.needsBuildLocked(now)
	c.mu.Unlock()

	if need {
		c.maybeBuild(now)
	}
	return s
}

// swapBoundary is when a freshly detected change should be adopted in old's timeline: the end of
// the programme spanning the most recent swap window if we are still inside it, else the end of
// the programme spanning the next window. Holding to the programme end avoids cutting a viewer
// mid-programme; deferring past a consumed window avoids adopting a change the moment it appears.
func (c *Channel) swapBoundary(old *Schedule, now time.Time) time.Time {
	last := time.Date(now.Year(), now.Month(), now.Day(), c.swapHour, c.swapMin, 0, 0, now.Location())
	if last.After(now) {
		last = last.AddDate(0, 0, -1)
	}
	if boundary := programmeEnd(old, last); now.Before(boundary) {
		return boundary
	}
	return programmeEnd(old, last.AddDate(0, 0, 1))
}

// programmeEnd is when the programme airing at t finishes in s's looping timeline, or t itself
// when s cannot be resolved.
func programmeEnd(s *Schedule, t time.Time) time.Time {
	if _, _, _, b, ok := locate(s, t); ok {
		return b
	}
	return t
}

// WarmUp builds the schedule synchronously when it is not yet ready, so the first viewer
// never waits on a cold build. It is a no-op once the channel is built and current, and
// honours ctx so a shutdown can cancel an in-progress build.
func (c *Channel) WarmUp(ctx context.Context, now time.Time) {
	c.mu.Lock()
	c.ensureLoadedLocked()
	need := c.needsBuildLocked(now)
	c.mu.Unlock()

	if !need || !c.building.CompareAndSwap(false, true) {
		return
	}
	defer c.building.Store(false)
	c.build(ctx, now)
}

// logCtx carries the channel identity the same way the streaming path does, so schedule
// logs show the familiar provider_name/channel_name fields instead of internal keys.
func (c *Channel) logCtx(ctx context.Context) context.Context {
	ctx = ctxutil.WithProviderName(ctx, c.playlist)
	return ctxutil.WithChannelName(ctx, c.name)
}

func (c *Channel) ensureLoadedLocked() {
	if c.loaded {
		return
	}
	if s, err := loadSchedule(c.stateDir, c.id); err != nil {
		logging.Error(c.logCtx(context.Background()), err, "failed to load channel schedule")
	} else {
		c.schedule = s
		// Restore a deferred change's armed swap boundary so a restart before it keeps the
		// original swap time rather than re-deferring to the next window.
		if s != nil && s.PendingSwapAt != 0 {
			c.pendingSwapAt = time.Unix(0, s.PendingSwapAt)
		}
	}
	c.loaded = true
}

func (c *Channel) needsBuildLocked(now time.Time) bool {
	if c.schedule == nil || c.dirty || (c.refresh > 0 && now.Sub(c.lastBuilt) >= c.refresh) {
		return true
	}
	// A deferred change arms a rebuild at its swap window so it is adopted there rather than at
	// the next refresh tick.
	return !c.pendingSwapAt.IsZero() && !now.Before(c.pendingSwapAt)
}

func (c *Channel) markDirty(now time.Time) {
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()
	c.maybeBuild(now)
}

func (c *Channel) maybeBuild(now time.Time) {
	if !c.building.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer c.building.Store(false)
		c.build(context.Background(), now)
	}()
}

func (c *Channel) build(ctx context.Context, now time.Time) {
	ctx = c.logCtx(ctx)

	c.mu.Lock()
	old := c.schedule
	pendingSwapAt := c.pendingSwapAt
	c.dirty = false // a change arriving mid-build re-sets dirty
	c.mu.Unlock()

	scan, err := scanContent(c.sources, c.extensions, c.order, c.seasonPatterns, c.episodePatterns, c.filler)
	if err != nil {
		logging.Error(ctx, err, "failed to scan channel sources")
		c.mu.Lock()
		c.dirty = true
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	c.lastBuilt = now
	c.mu.Unlock()

	if old != nil && scan.fingerprint == old.Fingerprint {
		c.setPendingSwap(old, time.Time{})
		logging.Info(ctx, "channel schedule unchanged", "items", len(old.Items))
		return
	}

	// A change is held until its swap boundary unless a removed file would air before then (the
	// running transcode would reopen a deleted file). Both decisions use the cheap scan, so the
	// probe pass below only runs when the change is actually adopted.
	if old != nil && !old.isEmpty() {
		// present holds content and filler paths alike: old.Items has both, so omitting filler
		// would flag every filler clip as removed and force an urgent swap on any change.
		present := make(map[string]struct{}, len(scan.files)+len(scan.fillerFiles))
		for _, f := range scan.files {
			present[f.path] = struct{}{}
		}
		for _, f := range scan.fillerFiles {
			present[f.path] = struct{}{}
		}

		// adoptBoundary is when this change is adopted: an already-deferred change keeps its armed
		// boundary (pendingSwapAt), a freshly detected one computes a new one. The urgent check
		// shares it, so a removal can never cut in before the schedule would have swapped anyway.
		adoptBoundary := pendingSwapAt
		if adoptBoundary.IsZero() {
			adoptBoundary = c.swapBoundary(old, now)
		}

		air, removed := earliestRemovedAir(old, present, now)
		urgent := removed && air.Before(adoptBoundary)
		reached := !now.Before(adoptBoundary)

		switch {
		case urgent:
			logging.Info(ctx, "removed file airs before swap boundary, adopting schedule early",
				"airs_at", air.Format(time.RFC3339))
		case reached:
			logging.Info(ctx, "adopting deferred channel schedule change at swap window")
		default:
			c.setPendingSwap(old, adoptBoundary)
			logging.Info(ctx, "channel schedule change deferred", "swap_at", adoptBoundary.Format(time.RFC3339))
			return
		}
	}

	logging.Info(ctx, "building channel schedule", "sources", c.sources)
	started := time.Now()
	s := buildSchedule(ctx, c.prober, c.id, scan, c.sources, c.order, c.seasonPatterns, c.episodePatterns, c.filler, old, now)

	if len(s.Items) == 0 {
		logging.Error(ctx, errors.New("no playable items"),
			"channel schedule is empty, channel will serve a placeholder",
			"sources", c.sources, "extensions", c.extensions)
	} else {
		logging.Info(ctx, "channel schedule built",
			"items", len(s.Items), "took", time.Since(started).Round(time.Millisecond))
	}

	if s.isEmpty() && old != nil && !old.isEmpty() {
		// Emptied sources can't be acted on; drop any armed swap so we don't rebuild on every
		// access until files reappear. A genuine later change re-arms via refresh or markDirty.
		c.setPendingSwap(old, time.Time{})
		return
	}

	c.mu.Lock()
	c.schedule = s
	c.pendingSwapAt = time.Time{}
	c.mu.Unlock()

	// The fingerprint always differs here (an unchanged scan returned above), so always persist.
	if err := saveSchedule(c.stateDir, s); err != nil {
		logging.Error(ctx, err, "failed to persist channel schedule")
	}
}

// setPendingSwap records the armed swap time (zero for none) in memory and persists it so the
// deferral survives a restart. The persisted marker is written to a copy: the published active
// schedule is immutable, so concurrent readers never see a torn write. The save is skipped when
// the armed time is unchanged.
func (c *Channel) setPendingSwap(active *Schedule, at time.Time) {
	c.mu.Lock()
	changed := !c.pendingSwapAt.Equal(at)
	c.pendingSwapAt = at
	c.mu.Unlock()

	if !changed {
		return
	}
	persisted := *active
	persisted.PendingSwapAt = 0
	if !at.IsZero() {
		persisted.PendingSwapAt = at.UnixNano()
	}
	if err := saveSchedule(c.stateDir, &persisted); err != nil {
		logging.Error(c.logCtx(context.Background()), err, "failed to persist swap time")
	}
}
