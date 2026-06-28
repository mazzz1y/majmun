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

Playout runs your command **once per file**. A supervisor resolves the channel's current file from its
[persisted schedule](./channels.md#how-it-works), runs the command for that file, and advances to the next when it
finishes — looping forever. Every run writes into one shared HLS directory, so viewers get a single continuous stream.
Per-file invocation is what lets the command adapt to each file via the
[`Playout.*` variables](#reserved-template-variables); a single whole-schedule process could not.

The command takes one input and produces one output:

- **Input:** `{{ .Playout.Input }}`, seeked to `{{ .Playout.Offset }}` (seconds). The file's probed parameters
  (`VideoCodec`, `Width`, `AudioLanguages`, …) are exposed too, so the command can pick its decoder, scaling, or audio
  track per file — see [Reserved Template Variables](#reserved-template-variables).
- **Output:** a rolling HLS stream at `{{ .Stream.PlaylistPath }}` with segments at `{{ .Stream.SegmentPath }}`.

Any command that does this works; the FFmpeg flags are up to you. The [next section](#what-a-command-must-take-care-of)
lists what a correct command must handle.

!!! warning "No default command"

    There is **no default `command`** — a transcode that keeps up in realtime depends on your hardware (CPU vs. a GPU
    encoder like NVENC/VAAPI/QSV), so majmun cannot pick one for you. If channels are configured but no `command` is set
    (at the global, playlist, or channel level), startup fails with an error. Start from a ready-to-use command in
    [Examples → Playout](../examples/playout.md) and adjust it to your hardware.

### What a command must take care of

A correct command handles all of these.

- **Realtime pacing** — emit ~1 second of video per second of wall time (`-re`). Without it the command races through
  the file as fast as it can encode, flooding the disk with segments.
- **Normalization** — scale/letterbox every source to one `width`×`height`×`fps` canvas and normalize audio, so the
  stream stays uniform across files. Skip this only if all sources are already uniform.
- **Appending to the live playlist** — each file's process writes into the same HLS directory, so the command must
  append rather than truncate (`-hls_flags append_list+omit_endlist`) and mark a discontinuity at the join
  (`discont_start`).
- **Segment alignment** — keyframes on segment boundaries, so joins are fast and segments uniform.
- **Buffering** — realtime pacing leaves only ~one segment of headroom, and majmun waits for `init_segments` segments
  before serving. `-readrate_initial_burst` emits the first few seconds at full speed to build that buffer without
  delaying start.

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
  order: sequential
  refresh_interval: 30m
  epg_duration: 1w
  schedule_swap_at: "04:00"
  metadata:
    title: '{{ .Probe.Title | default .File.Name }}'
    description: '{{ .Probe.Description }}'
    category: '{{ .Probe.Category }}'
  filler:
    sources: []
    every: 1h
    max_duration: 0s
    order: shuffle
    metadata:
      title: Advertising

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
| `command`            | [`Command`](./shared/command.md)              | **Yes**  | Transcoder command. No default — see [How It Works](#how-it-works).                                      |
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
| `order`            | `string`                           | No       | Playback order: `sequential` (episode order), `shuffle` (stable random), `interleave` (round-robin episodes across shows; shorter shows finish first), or `spread` (distribute each show's episodes evenly across the whole timeline so all shows run start-to-finish). Default `sequential`. |
| `season_patterns`  | `[]regex`                          | No       | Patterns matching season folder names, used to group shows (for `interleave`/`spread`) and to fill `{{ .Probe.Season }}`. Defaults cover `Season N` / `Сезон N` / `S01` and bare numbers. |
| `episode_patterns` | `[]regex`                          | No       | Patterns to extract season/episode numbers from the tag and filename, used for sorting and metadata (`{{ .Probe.Season }}` / `{{ .Probe.Episode }}`). Two capture groups yield `(season, episode)`, one yields the episode. Defaults cover `S01E05`, `1x05`, `ep05`, a leading number followed by a dot (`20. Title`), and similar. |
| `refresh_interval` | [`duration`](./shared/duration.md) | No       | How often `sources` is re-scanned for added/removed files (see [adopting changes](./channels.md#picking-up-file-changes)). Default `30m`; `0` disables re-scanning.                          |
| `epg_duration`     | [`duration`](./shared/duration.md) | No       | How far into the future the EPG is generated. Default `1w`.                                                                                                                               |
| `schedule_swap_at` | `string` (`HH:MM`)                 | No       | Local time of day after which a changed file set is adopted by the live stream — deferred further to the end of the programme then playing, so a show is never cut off mid-way. Default `04:00`. |
| `metadata`         | [`Metadata`](#metadata-templates)  | No       | Go templates building the EPG `title`, `description`, and `category` per file. Defaults reproduce the [container-tag behavior](./channels.md#epg-metadata).                                       |
| `filler`           | [`Filler`](#filler)                | No       | Inject filler clips between content. Off unless `sources` is set. See [Filler](#filler).                                                                                                         |

!!! info "Season/episode detection"

    Season and episode numbers are resolved per file in this order, stopping at the first hit:

    1. **Container tags** — `episode_id` / `episode_sort` / `episode`, parsed with `episode_patterns`.
    2. **Filename** — parsed with `episode_patterns` (e.g. `S01E05`, `1x05`, `20. Title`).
    3. **Season folder** — the season number from the `season_patterns` folder fills the season if still unset.

    A file with no detectable numbers keeps season/episode `0`; ordering then falls back to natural filename sort within the directory.

### Metadata Templates

`metadata` holds Go templates (with [sprig](https://masterminds.github.io/sprig/) functions) that build each file's EPG
fields. They are evaluated when the guide is generated, so editing them takes effect on the next EPG request without a
rebuild. Each subkey is optional; omitted keys use the defaults below, which match the
[container-tag behavior](./channels.md#epg-metadata).

| Field         | Default                                    | Description                                  |
| ------------- | ------------------------------------------ | -------------------------------------------- |
| `title`       | `{{ .Probe.Title \| default .File.Name }}` | EPG programme title.                         |
| `description` | `{{ .Probe.Description }}`                  | EPG programme description (`<desc>`).         |
| `category`    | `{{ .Probe.Category }}`                     | EPG programme category (`<category>`).        |

`date` and `episode` are derived in fixed ways and are **not** templatable. If a template fails to render for a file,
the raw tag value is used instead and the error is logged.

Templates receive:

| Variable | Description |
| --- | --- |
| `{{ .Probe.Title }}` | Raw `title` tag (may be empty). |
| `{{ .Probe.Description }}` | Raw `description`/`synopsis`/`summary`/`comment` tag. |
| `{{ .Probe.Category }}` | Raw `genre` tag. |
| `{{ .Probe.Date }}` | Normalized date (`YYYYMMDD`) when known. |
| `{{ .Probe.Season }}` / `{{ .Probe.Episode }}` | Parsed season/episode numbers (`0` when unknown). |
| `{{ .Probe.VideoCodec }}`, `{{ .Probe.Width }}`, `{{ .Probe.Height }}`, … | The file's probed media parameters (same set as the `Playout.*` command variables). |
| `{{ .File.Path }}` | Absolute file path. |
| `{{ .File.Rel }}` | Path relative to the source root that contains the file, e.g. `Show/S01E05.mkv`. |
| `{{ .File.RelNoExt }}` | Path relative to the source root, without the file extension, e.g. `Show/Season 1/20. Title`. |
| `{{ .File.Name }}` | File name without its extension. |
| `{{ .File.Source }}` | The configured source root that contains the file. |
| `{{ .File.SourceBase }}` | Base name of the source root, e.g. `My Show` for source `/media/series/My Show`. Useful as the series name when each source is a single show. |

For a channel that merges several shows, prefix the show folder so episodes are distinguishable:

```yaml
playout:
  metadata:
    title: '{{ .File.Rel | splitList "/" | first }} — {{ .Probe.Title | default .File.Name }}'
```

### Filler

`filler` inserts breaks of clips from `sources` between content. Clips play through the same `command` as content. Set
`sources` to enable filler; everything else is optional.

| Field          | Type                               | Required | Description                                                                                                      |
| -------------- | ---------------------------------- | -------- | --------------------------------------------------------------------------------------------------------------- |
| `sources`      | `[]string`                         | **Yes**  | Directories or files holding filler clips. Scanned with the same `extensions` as content.                       |
| `every`        | [`duration`](./shared/duration.md) | No       | Break after roughly this much content playtime (rounded up to a whole item). Default `1h`; excludes `every_count`. |
| `every_count`  | `int`                              | No       | Break after every this many content items. Excludes `every`.                                                    |
| `max_duration` | [`duration`](./shared/duration.md) | No       | Cap on filler time per break. `0` (default) plays one clip per break.                                           |
| `order`        | `string`                           | No       | Order clips are drawn in: `shuffle` (default) or `sequential`.                                                  |
| `metadata`     | [`Metadata`](#metadata-templates)  | No       | EPG `title` (default `Advertising`) and `category` for breaks.                                                  |

In the EPG, each break shows as a single programme (e.g. `Advertising 18:50–18:53`) with no season/episode/description.

If `sources` holds more clips than the breaks in one loop can play, the pool rotates: each schedule rebuild (when files
or config change) resumes from where the last one stopped, so every clip eventually airs.

### Reserved Template Variables

The playout command receives these runtime variables (also as `MAJMUN_*` environment variables) — see
[Command → Reserved Variables](./shared/command.md#reserved-variables) for the full reference:

| Variable | Environment variable | Description |
| --- | --- | --- |
| `{{ .Playout.Input }}` | `MAJMUN_PLAYOUT_INPUT` | Path of the file to play now. |
| `{{ .Playout.Offset }}` | `MAJMUN_PLAYOUT_OFFSET` | Seek position (seconds) into the file for the live point. Empty at the file's start. |
| `{{ .Playout.VideoCodec }}` | `MAJMUN_PLAYOUT_VIDEO_CODEC` | The file's video codec (e.g. `h264`, `hevc`), for selecting a decoder. Empty if unknown. |
| `{{ .Playout.Width }}` / `{{ .Playout.Height }}` | `MAJMUN_PLAYOUT_WIDTH` / `MAJMUN_PLAYOUT_HEIGHT` | Coded video dimensions in pixels, as stored in the file. |
| `{{ .Playout.AspectWidth }}` | `MAJMUN_PLAYOUT_ASPECT_WIDTH` | `Width` corrected for the sample aspect ratio: the square-pixel display width (even-rounded). Equals `Width` for square-pixel sources; larger/smaller for anamorphic ones. Use as the `scale_vaapi` width to bake the aspect into the pixels without a software `setsar`/`setdar`. |
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

## Examples

See [Examples → Playout](../examples/playout.md) for full, ready-to-use configs: basic channels, the cascade
override, a software transcode, a logo watermark, and hardware-accelerated (NVENC / VAAPI / QSV) variants.
