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

type Config struct {
	Playlist       string
	Name           string
	Sources        []string
	Extensions     []string
	Order          string
	SeasonPatterns []*regexp.Regexp
	Refresh        time.Duration
	EPGDuration    time.Duration
	SwapHour       int
	SwapMin        int
	StateDir       string

	TitleTemplate       *template.Template
	DescriptionTemplate *template.Template
	CategoryTemplate    *template.Template
}

type Channel struct {
	// id is the channel's hashid, shared with app.Channel.ID(): schedule file name,
	// the schedule JSON's "channel" field, and the tvg-id all carry this same token.
	id             string
	playlist       string
	name           string
	sources        []string
	extensions     []string
	order          string
	seasonPatterns []*regexp.Regexp
	refresh        time.Duration
	epgDuration    time.Duration
	swapHour       int
	swapMin        int
	stateDir       string
	prober         prober

	titleTmpl       *template.Template
	descriptionTmpl *template.Template
	categoryTmpl    *template.Template

	mu        sync.Mutex
	schedule  *Schedule
	loaded    bool
	dirty     bool
	lastBuilt time.Time
	building  atomic.Bool

	pending   *Schedule
	promoteAt time.Time
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
		refresh:         cfg.Refresh,
		epgDuration:     cfg.EPGDuration,
		swapHour:        cfg.SwapHour,
		swapMin:         cfg.SwapMin,
		stateDir:        cfg.StateDir,
		prober:          ffprobeProber{},
		titleTmpl:       cfg.TitleTemplate,
		descriptionTmpl: cfg.DescriptionTemplate,
		categoryTmpl:    cfg.CategoryTemplate,
	}
}

// current returns the servable schedule without blocking. It is nil while the first build
// runs (callers serve a placeholder). A background build is triggered when the channel is
// unbuilt, marked dirty by a deleted file, or past its refresh interval.
func (c *Channel) current(now time.Time) *Schedule {
	c.mu.Lock()
	c.ensureLoadedLocked()
	promoted := c.promotePendingLocked(now)
	s := c.schedule
	need := c.needsBuildLocked(now)
	c.mu.Unlock()

	if promoted != nil {
		if err := saveSchedule(c.stateDir, promoted); err != nil {
			logging.Error(c.logCtx(context.Background()), err, "failed to persist channel schedule")
		}
	}
	if need {
		c.maybeBuild(now)
	}
	return s
}

// promotePendingLocked adopts a deferred schedule once its swap time has arrived and the
// programme playing at that time has finished. It returns the newly adopted schedule (to be
// persisted by the caller outside the lock) or nil. Callers must hold c.mu.
func (c *Channel) promotePendingLocked(now time.Time) *Schedule {
	if c.pending == nil || now.Before(c.promoteAt) {
		return nil
	}
	if c.schedule != nil {
		// Boundary is anchored at promoteAt (the programme airing at the swap time), not now,
		// so a long gap since promoteAt still promotes (the boundary is then in the past).
		if _, _, _, boundary, ok := locate(c.schedule, c.promoteAt); ok && now.Before(boundary) {
			return nil
		}
	}
	s := c.pending
	c.schedule = s
	c.pending = nil
	c.promoteAt = time.Time{}
	return s
}

func (c *Channel) nextSwap(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), c.swapHour, c.swapMin, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
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
	}
	c.loaded = true
}

func (c *Channel) needsBuildLocked(now time.Time) bool {
	return c.schedule == nil || c.dirty || (c.refresh > 0 && now.Sub(c.lastBuilt) >= c.refresh)
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
	c.dirty = false // consume the signal now; a change arriving mid-build re-sets it
	c.mu.Unlock()

	logging.Info(ctx, "building channel schedule", "sources", c.sources)
	started := time.Now()

	s, err := buildSchedule(ctx, c.prober, c.id, c.sources, c.extensions, c.order, c.seasonPatterns, old, now)
	if err != nil {
		logging.Error(ctx, err, "failed to build channel schedule")
		c.mu.Lock()
		c.dirty = true // build failed; re-arm so the next access retries
		c.mu.Unlock()
		return
	}

	switch {
	case len(s.Items) == 0:
		logging.Error(ctx, errors.New("no playable items"),
			"channel schedule is empty, channel will serve a placeholder",
			"sources", c.sources, "extensions", c.extensions)
	case old != nil && s.Fingerprint == old.Fingerprint:
		logging.Info(ctx, "channel schedule unchanged", "items", len(s.Items))
	default:
		logging.Info(ctx, "channel schedule built",
			"items", len(s.Items), "took", time.Since(started).Round(time.Millisecond))
	}

	c.mu.Lock()
	c.lastBuilt = now

	changed := old != nil && !old.isEmpty() && s.Fingerprint != old.Fingerprint && !s.isEmpty()
	if changed {
		swapAt := c.nextSwap(now)
		// Promotion holds until the programme spanning swapAt finishes, so a removal must be
		// compared against that boundary, not swapAt, or it could air live before adoption.
		boundary := swapAt
		if _, _, _, b, ok := locate(old, swapAt); ok {
			boundary = b
		}
		air, removed := earliestRemovedAir(old, s, now)
		if !removed || !air.Before(boundary) {
			// Keep an already-scheduled swap time so repeated refresh rebuilds of the same
			// change do not keep pushing the promotion to the next day.
			if c.pending == nil || c.pending.Fingerprint != s.Fingerprint {
				c.promoteAt = swapAt
				logging.Info(ctx, "channel schedule change deferred", "swap_at", c.promoteAt.Format(time.RFC3339))
			}
			c.pending = s
			c.mu.Unlock()
			return
		}
		logging.Info(ctx, "removed file airs before swap boundary, adopting schedule early",
			"airs_at", air.Format(time.RFC3339), "boundary", boundary.Format(time.RFC3339))
	}

	if s.isEmpty() && old != nil && !old.isEmpty() {
		c.mu.Unlock()
		return
	}

	c.pending = nil
	c.promoteAt = time.Time{}
	c.schedule = s
	c.mu.Unlock()

	if old == nil || s.Fingerprint != old.Fingerprint {
		if err := saveSchedule(c.stateDir, s); err != nil {
			logging.Error(ctx, err, "failed to persist channel schedule")
		}
	}
}
