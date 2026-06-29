package channelgen

type Item struct {
	File           string   `json:"file"`
	Title          string   `json:"title"`
	Show           string   `json:"show,omitempty"`
	Description    string   `json:"description,omitempty"`
	Category       string   `json:"category,omitempty"`
	Date           string   `json:"date,omitempty"`
	Season         int      `json:"season,omitempty"`
	Episode        int      `json:"episode,omitempty"`
	EpisodeTag     string   `json:"episode_tag,omitempty"`
	IsFiller       bool     `json:"is_filler,omitempty"`
	Size           int64    `json:"size"`
	MTime          int64    `json:"mtime"`
	Duration       float64  `json:"duration"`
	VideoCodec     string   `json:"video_codec,omitempty"`
	Width          int      `json:"width,omitempty"`
	Height         int      `json:"height,omitempty"`
	AspectWidth    int      `json:"aspect_width,omitempty"`
	PixelFormat    string   `json:"pixel_format,omitempty"`
	FrameRate      string   `json:"frame_rate,omitempty"`
	FieldOrder     string   `json:"field_order,omitempty"`
	AudioCodec     string   `json:"audio_codec,omitempty"`
	AudioChannels  int      `json:"audio_channels,omitempty"`
	SampleRate     int      `json:"sample_rate,omitempty"`
	AudioLanguages []string `json:"audio_languages,omitempty"`
}

type Schedule struct {
	// Channel is the owning channel's hashid; it doubles as the schedule file's basename.
	Channel     string `json:"channel"`
	Seed        int64  `json:"seed"`
	Fingerprint string `json:"fingerprint"`
	Anchor      int64  `json:"anchor"`
	Items       []Item `json:"items"`
	// FillerSeed shuffles the filler pool independently of content order, so filler.order: shuffle
	// is randomized even when content is not shuffled. Sticky across rebuilds, like Seed, to keep
	// the pool order (and thus the FillerStart cursor) stable.
	FillerSeed int64 `json:"filler_seed,omitempty"`
	// FillerStart is the filler-pool index the next rebuild begins from, so a pool larger than one
	// loop's breaks rotates through all clips over successive rebuilds.
	FillerStart   int   `json:"filler_start,omitempty"`
	PendingSwapAt int64 `json:"pending_swap_at,omitempty"`
}

func (s *Schedule) total() float64 {
	var sum float64
	for _, it := range s.Items {
		sum += it.Duration
	}
	return sum
}

func (s *Schedule) isEmpty() bool {
	return len(s.Items) == 0 || s.total() <= 0
}

func (s *Schedule) probeCache() map[probeKey]probeResult {
	cache := make(map[probeKey]probeResult, len(s.Items))
	for _, it := range s.Items {
		cache[probeKey{file: it.File, size: it.Size, mtime: it.MTime}] = probeResult{
			Duration:       it.Duration,
			Title:          it.Title,
			Show:           it.Show,
			Description:    it.Description,
			Category:       it.Category,
			Date:           it.Date,
			Season:         it.Season,
			Episode:        it.Episode,
			EpisodeTag:     it.EpisodeTag,
			VideoCodec:     it.VideoCodec,
			Width:          it.Width,
			Height:         it.Height,
			AspectWidth:    it.AspectWidth,
			PixelFormat:    it.PixelFormat,
			FrameRate:      it.FrameRate,
			FieldOrder:     it.FieldOrder,
			AudioCodec:     it.AudioCodec,
			AudioChannels:  it.AudioChannels,
			SampleRate:     it.SampleRate,
			AudioLanguages: it.AudioLanguages,
		}
	}
	return cache
}

type probeKey struct {
	file  string
	size  int64
	mtime int64
}
