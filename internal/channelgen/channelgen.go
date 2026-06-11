package channelgen

import (
	"context"
	"errors"
	"majmun/internal/ctxutil"
	"majmun/internal/hashid"
	"majmun/internal/logging"
	"sync"
	"sync/atomic"
	"time"
)

type Channel struct {
	// id is the channel's hashid, shared with app.Channel.ID(): schedule file name,
	// the schedule JSON's "channel" field, and the tvg-id all carry this same token.
	id          string
	playlist    string
	name        string
	sources     []string
	extensions  []string
	randomOrder bool
	refresh     time.Duration
	epgDuration time.Duration
	stateDir    string
	prober      prober

	mu        sync.Mutex
	schedule  *Schedule
	loaded    bool
	dirty     bool
	lastBuilt time.Time
	building  atomic.Bool
}

func NewChannel(playlist, name string, sources, extensions []string, randomOrder bool, refresh, epgDuration time.Duration, stateDir string) *Channel {
	return &Channel{
		id:          hashid.New(playlist, name),
		playlist:    playlist,
		name:        name,
		sources:     sources,
		extensions:  extensions,
		randomOrder: randomOrder,
		refresh:     refresh,
		epgDuration: epgDuration,
		stateDir:    stateDir,
		prober:      ffprobeProber{},
	}
}

// current returns the servable schedule without blocking. It is nil while the first build
// runs (callers serve a placeholder). A background build is triggered when the channel is
// unbuilt, marked dirty by a deleted file, or past its refresh interval.
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

	s, err := buildSchedule(ctx, c.prober, c.id, c.sources, c.extensions, c.randomOrder, old, now)
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

	if old == nil || s.Fingerprint != old.Fingerprint {
		if err := saveSchedule(c.stateDir, s); err != nil {
			logging.Error(ctx, err, "failed to persist channel schedule")
		}
	}

	c.mu.Lock()
	c.schedule = s
	c.lastBuilt = now
	c.mu.Unlock()
}
