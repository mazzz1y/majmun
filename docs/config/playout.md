# Playout

`playout` configures generated [channels](./channels.md) — your local media played as 24/7 live TV. It holds two
things:

- **Transcode** — the FFmpeg command that turns local media into a continuous live stream.
- **Scheduling & listing** — refresh interval, daily swap time, logo, state directory, and more.

Settings cascade **Global ➡ Playlist ➡ Channel**: define them once at the top, and a channel overrides only what
differs. A channel itself carries just `name`, `sources`, `fields`, and its `playout` override.

Like the proxy [segmenter](./proxy/segmenter.md), playout runs **once per channel** and is shared by all viewers (the
first viewer's config wins). The difference is the input: the segmenter pulls a remote stream, while playout reads your
local files.

## How It Works

Playout runs **one FFmpeg process per file**. A supervisor resolves the channel's current file from its
[persisted schedule](./channels.md#how-it-works), launches the command for that single file, and when it finishes
advances to the next file — looping the schedule forever. All processes write into one shared HLS directory, so viewers
read a single continuous stream with a brief discontinuity at each file boundary. The supervisor runs until the last
viewer leaves.

Per-file invocation lets the command tailor itself to each file using the per-file variables (e.g. stream mapping or
decoder choice) — decisions a single whole-schedule process cannot make. How it uses them is up to you.

The command reads one file and writes a live HLS stream on disk:

- **Input:** `{{ .Playout.Input }}` — the path of the file to play now, with `{{ .Playout.Offset }}` the seek position
  (seconds) for the live point. The file's probed media parameters (`VideoCodec`, `Width`, `Height`, `PixelFormat`,
  `FrameRate`, `FieldOrder`, `AudioCodec`, `AudioChannels`, `SampleRate`, `AudioLanguages`) are exposed too, so the
  command can adapt its decoder, scaling, deinterlace, or audio track per file — see
  [Reserved Template Variables](#reserved-template-variables).
- **Output:** `{{ .Stream.SegmentPath }}` (segment files like `/tmp/.../seg_%05d.ts`) and `{{ .Stream.PlaylistPath }}`
  (a live, rolling HLS playlist — no `EXT-X-ENDLIST`, old segments deleted).

Two things matter here:

- **Pace output at realtime.** Without it a command encodes as fast as possible, racing through the file and filling the
  disk with segments. Realtime pacing (`-re`) emits ~1 second of video per second of wall time.
- **Startup.** majmun starts serving viewers once `init_segments` segments exist (or `ready_timeout` passes).

Any command that satisfies this — reads the file, writes a paced live HLS stream — works; the exact FFmpeg flags are
up to you.

### What a command must take care of

The default command handles all of these; a custom command must too.

- **Normalization** — scale/letterbox every source to one `width`×`height`×`fps` canvas and normalize audio, so the
  stream stays uniform across files. Skip this only if all sources are already uniform.
- **Appending to the live playlist** — each file's process writes into the same HLS directory, so the command must
  append rather than truncate (`-hls_flags append_list+omit_endlist`) and mark a discontinuity at the join
  (`discont_start`).
- **Segment alignment** — keyframes on segment boundaries, so joins are fast and segments uniform.
- **Buffering** — realtime pacing leaves only ~one segment of headroom. `-readrate_initial_burst` emits the first
  `initial_burst` seconds at full speed to build a buffer without delaying start (see [Startup Buffer](#startup-buffer)).

## YAML Structure

```yaml
playout:
  command: []
  template_variables: []
  env_variables: []
  init_segments: 4
  ready_timeout: 30s
  state_dir: state
  logo: ""
  extensions: []
  random_order: false
  refresh_interval: 30m
  epg_duration: 1w
  schedule_swap_at: "04:00"

playlists:
  - name: local
    playout: {} # playlist-level overrides (any subset of the above)
    channels:
      - name: cartoons
        sources: [/media/cartoons]
        playout: {} # channel-level overrides
```

## Fields

### Transcode

| Field                | Type                                          | Required | Description                                                                                             |
| -------------------- | --------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------------- |
| `command`            | [`Command`](./shared/command.md)              | **Yes**  | Transcoder command. No default — see [Choosing a Command](#choosing-a-command).                          |
| `template_variables` | [`[]NameValue`](./shared/name-value.md)       | No       | User variables for the command. Merged by name across cascade levels.                                  |
| `env_variables`      | [`[]NameValue`](./shared/name-value.md)       | No       | Environment variables for the command. Merged by name across cascade levels.                            |
| `init_segments`      | `int`                                         | No       | Number of segments that must exist before clients can start reading (default: `4`). Must be at least 1. |
| `ready_timeout`      | [`duration`](./shared/duration.md)            | No       | Maximum time to wait for the initial segments to become available (default: `30s`).                     |

### Scheduling & Listing

| Field              | Type                               | Required | Description                                                                                                                                                                                |
| ------------------ | ---------------------------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `state_dir`        | `string`                           | No       | Directory where channel schedules are persisted. Default `state`.                                                                                                                          |
| `logo`             | `string`                           | No       | Channel logo: an http(s) URL or a local file path. Available in commands as `{{ .Channel.Logo }}`.                                                                                         |
| `extensions`       | `[]string`                         | No       | Video file extensions to include. Default: `mkv, mp4, avi, mov, m4v, ts, webm, mpg, mpeg, flv, wmv`.                                                                                       |
| `random_order`     | `bool`                             | No       | When `true`, files play in a stable shuffled order; otherwise [episode order](./channels.md#playback-order). Default `false`.                                                              |
| `refresh_interval` | [`duration`](./shared/duration.md) | No       | How often `sources` is re-scanned for added/removed files (see [adopting changes](./channels.md#picking-up-file-changes)). Default `30m`; `0` disables re-scanning.                          |
| `epg_duration`     | [`duration`](./shared/duration.md) | No       | How far into the future the EPG is generated. Default `1w`.                                                                                                                               |
| `schedule_swap_at` | `string` (`HH:MM`)                 | No       | Local time of day after which a changed file set is adopted by the live stream — deferred further to the end of the programme then playing, so a show is never cut off mid-way. Default `04:00`. |

### Choosing a Command

There is **no default `command`** — a transcode that keeps up in realtime depends on your hardware (CPU vs. a
GPU encoder like NVENC/VAAPI/QSV), so majmun cannot pick one for you. If channels are configured but no `command` is set
(at the global, playlist, or channel level), startup fails with an error.

Start from a ready-to-use command in [Examples → Playout](../examples/playout.md) and adjust it to your hardware.

### Reserved Template Variables

The playout command receives these runtime variables (also as `MAJMUN_*` environment variables) — see
[Command → Reserved Variables](./shared/command.md#reserved-variables) for the full reference:

| Variable | Environment variable | Description |
| --- | --- | --- |
| `{{ .Playout.Input }}` | `MAJMUN_PLAYOUT_INPUT` | Path of the file to play now. |
| `{{ .Playout.Offset }}` | `MAJMUN_PLAYOUT_OFFSET` | Seek position (seconds) into the file for the live point. Empty at the file's start. |
| `{{ .Playout.VideoCodec }}` | `MAJMUN_PLAYOUT_VIDEO_CODEC` | The file's video codec (e.g. `h264`, `hevc`), for selecting a decoder. Empty if unknown. |
| `{{ .Playout.Width }}` / `{{ .Playout.Height }}` | `MAJMUN_PLAYOUT_WIDTH` / `MAJMUN_PLAYOUT_HEIGHT` | Video dimensions in pixels. |
| `{{ .Playout.PixelFormat }}` | `MAJMUN_PLAYOUT_PIXEL_FORMAT` | Video pixel format (e.g. `yuv420p`). |
| `{{ .Playout.FrameRate }}` | `MAJMUN_PLAYOUT_FRAME_RATE` | Video frame rate as a fraction (e.g. `30000/1001`). |
| `{{ .Playout.FieldOrder }}` | `MAJMUN_PLAYOUT_FIELD_ORDER` | Field order (`progressive`, `tt`, `bb`, …), for deinterlace decisions. |
| `{{ .Playout.AudioCodec }}` | `MAJMUN_PLAYOUT_AUDIO_CODEC` | First audio stream codec (e.g. `aac`). |
| `{{ .Playout.AudioChannels }}` | `MAJMUN_PLAYOUT_AUDIO_CHANNELS` | First audio stream channel count. |
| `{{ .Playout.SampleRate }}` | `MAJMUN_PLAYOUT_SAMPLE_RATE` | First audio stream sample rate in Hz. |
| `{{ .Playout.AudioLanguages }}` | `MAJMUN_PLAYOUT_AUDIO_LANGUAGES` | Space-separated language of each audio stream, in order (`und` when untagged), e.g. `eng rus und`. |
| `{{ .Stream.SegmentPath }}` | `MAJMUN_STREAM_SEGMENT_PATH` | Output segment file path pattern. |
| `{{ .Stream.PlaylistPath }}` | `MAJMUN_STREAM_PLAYLIST_PATH` | Output HLS playlist path. |
| `{{ .Channel.Name }}` | `MAJMUN_CHANNEL_NAME` | Channel name. |
| `{{ .Channel.Logo }}` | `MAJMUN_CHANNEL_LOGO` | Channel logo path/URL, e.g. `-i "{{ .Channel.Logo }}"` for a watermark. |
| `{{ .Playlist.Name }}` | `MAJMUN_PLAYLIST_NAME` | Parent playlist name. |

## Startup Buffer

When a viewer connects, majmun waits for the transcode to produce `init_segments` segments (default `4`, about 8s at
2s each) before it starts serving. This head start matters: it gives the player a few seconds of video in reserve, so a
brief encoder slowdown doesn't immediately stall playback. The `initial_burst` setting lets the transcode render those
first segments faster than realtime, keeping the wait short.

Tuning:

- **Lower `init_segments`** → viewers start sooner, but with less buffer to absorb hiccups.
- **Raise `init_segments`** (and `initial_burst`) → more buffer, better on slow encoders, at the cost of a longer
  initial wait.

## Examples

See [Examples → Playout](../examples/playout.md) for full, ready-to-use configs: basic channels, the cascade
override, a software transcode, a logo watermark, and hardware-accelerated (NVENC / VAAPI / QSV) variants.
