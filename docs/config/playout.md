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

On each request, majmun resolves the channel's current position from its [persisted schedule](./channels.md#how-it-works),
writes an FFmpeg **concat list** to a scratch dir, and launches the command. It runs until the last viewer leaves.

The command reads one input and writes a live HLS stream on disk:

- **Input:** `{{ .Playout.Input }}` — a [concat demuxer](https://ffmpeg.org/ffmpeg-formats.html#concat-1) list: the
  full schedule, rotated so the currently-airing file is first and seeked to the live position. The files are raw media
  with mixed codecs, resolutions, and audio layouts.
- **Output:** `{{ .Stream.SegmentPath }}` (segment files like `/tmp/.../seg_%05d.ts`) and `{{ .Stream.PlaylistPath }}`
  (a live, rolling HLS playlist — no `EXT-X-ENDLIST`, old segments deleted).

Two things matter here:

- **Pace output at realtime.** The schedule loops forever, so a command that encodes as fast as possible would race
  through your whole library and fill the disk with segments. Realtime pacing (`-re`) emits ~1 second of video per
  second of wall time.
- **Startup.** majmun starts serving viewers once `init_segments` segments exist (or `ready_timeout` passes).

Any command that satisfies this — reads the input, writes a paced live HLS stream — works; the exact FFmpeg flags are
up to you.

### What a command must take care of

The default command handles all of these; a custom command must too.

- **Normalization** — scale/letterbox every source to one `width`×`height`×`fps` canvas and normalize audio, or the
  stream breaks at the first file that differs. Skip this only if all sources are already uniform.
- **Looping** — repeat the schedule forever (`-stream_loop -1`). Legacy containers (MPEG-PS/AVI) need
  `-af aresample=async=1:first_pts=0` to keep audio timestamps monotonic across loop wraps, or playback stalls after
  the first wrap.
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
| `{{ .Playout.Input }}` | `MAJMUN_PLAYOUT_INPUT` | The FFmpeg concat list (the rotated, seeked schedule). |
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
