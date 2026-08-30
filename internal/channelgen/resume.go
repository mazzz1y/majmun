package channelgen

import (
	"time"
)

// resumeCursor remembers where a viewer's catch-up session stopped, so a reconnect continues
// instead of replaying from the requested utc. It is tagged with that utc: a different one
// means the viewer picked another programme and the cursor no longer applies.
//
// Resume is frozen — the position does not advance while the viewer is away, so the shift
// grows by the whole gap. Once it exceeds the catch-up window the position has fallen outside
// the advertised rewind range, and the cursor is dropped rather than playing unrelated content.
type resumeCursor struct {
	utc      time.Time
	schedPos time.Time
}

func (c *Channel) ResumeShift(key string, requested, now time.Time) time.Duration {
	c.resumeMu.Lock()
	cur, ok := c.resume[key]
	c.resumeMu.Unlock()

	if !ok || !cur.utc.Equal(requested) {
		return now.Sub(requested)
	}
	shift := now.Sub(cur.schedPos)
	if window := c.CatchupWindow(now); shift > window {
		c.ClearResume(key)
		return now.Sub(requested)
	}
	return shift
}

func (c *Channel) SaveResume(key string, utc, schedPos time.Time) {
	c.resumeMu.Lock()
	defer c.resumeMu.Unlock()
	if c.resume == nil {
		c.resume = make(map[string]resumeCursor)
	}
	c.resume[key] = resumeCursor{utc: utc, schedPos: schedPos}
}

func (c *Channel) ClearResume(key string) {
	c.resumeMu.Lock()
	defer c.resumeMu.Unlock()
	delete(c.resume, key)
}
