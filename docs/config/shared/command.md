# Command

Several config sections run an external process from a `command`: the proxy [stream](../proxy/stream.md),
[segmenter](../proxy/segmenter.md) and [error handlers](../proxy/error.md), and the channel
[playout](../playout.md). They all share the same shape and templating rules, described here.

A command is configured with three fields, all optional:

| Field                | Type                                  | Description                                                          |
| -------------------- | ------------------------------------- | -------------------------------------------------------------------- |
| `command`            | [array or string](#forms)             | The program and its arguments.                                       |
| `template_variables` | [`[]NameValue`](./name-value.md)      | User variables available in the command as `{{ .name }}`.            |
| `env_variables`      | [`[]NameValue`](./name-value.md)      | Environment variables set for the process.                           |

## Forms

`command` is an array of strings: the program followed by its arguments, each a separate element. Every element
supports [Go template](https://pkg.go.dev/text/template) syntax with
[Sprig functions](https://masterminds.github.io/sprig/).

```yaml
command:
  - "ffmpeg"
  - "-v"
  - "{{ .log_level }}"
  - "-i"
  - "{{ .Stream.PlaylistPath }}"
```

The first element is the program; the rest are its arguments.

## Template Variables

Two kinds of variables are available inside a command:

- **User variables** — anything you define in `template_variables`, referenced as `{{ .name }}`. Sections that ship a
  default command document their own defaults (e.g. `width`, `fps`, `segment_duration`); override them by name. Where a
  section cascades (Global ➡ Playlist ➡ Channel), `template_variables` are merged by name across levels.
- **Reserved variables** — injected by majmun at runtime (see [below](#reserved-variables)).

## Reserved Variables

Majmun injects a set of namespaced variables at runtime. They are always available in the command (no need to declare
them) and are **also** exposed to the process as `MAJMUN_`-prefixed environment variables, so wrapper scripts can read
them without templating.

| Template variable            | Environment variable          | Available in            | Description                                           |
| ---------------------------- | ----------------------------- | ----------------------- | ----------------------------------------------------- |
| `{{ .Stream.URL }}`          | `MAJMUN_STREAM_URL`           | segmenter               | Upstream stream URL                                   |
| `{{ .Stream.SegmentPath }}`  | `MAJMUN_STREAM_SEGMENT_PATH`  | segmenter, playout      | Segment filename pattern (e.g. `/tmp/.../seg_%05d.ts`)|
| `{{ .Stream.PlaylistPath }}` | `MAJMUN_STREAM_PLAYLIST_PATH` | segmenter, stream, playout | HLS playlist path (e.g. `/tmp/.../stream.m3u8`)    |
| `{{ .Playout.Input }}`       | `MAJMUN_PLAYOUT_INPUT`        | playout                 | Path of the file to play now                          |
| `{{ .Playout.Offset }}`      | `MAJMUN_PLAYOUT_OFFSET`       | playout                 | Seek position (seconds) into the file; empty at start |
| `{{ .Playout.VideoCodec }}`  | `MAJMUN_PLAYOUT_VIDEO_CODEC`  | playout                 | The file's video codec (e.g. `h264`, `hevc`)          |
| `{{ .Playout.Width }}`       | `MAJMUN_PLAYOUT_WIDTH`        | playout                 | Coded video width in pixels (as stored)               |
| `{{ .Playout.Height }}`      | `MAJMUN_PLAYOUT_HEIGHT`       | playout                 | Video height in pixels                                |
| `{{ .Playout.AspectWidth }}` | `MAJMUN_PLAYOUT_ASPECT_WIDTH` | playout                 | `Width` corrected for the sample aspect ratio (square-pixel, even-rounded); use as the `scale_vaapi` width |
| `{{ .Playout.PixelFormat }}` | `MAJMUN_PLAYOUT_PIXEL_FORMAT` | playout                 | Video pixel format (e.g. `yuv420p`)                   |
| `{{ .Playout.FrameRate }}`   | `MAJMUN_PLAYOUT_FRAME_RATE`   | playout                 | Video frame rate as a fraction (e.g. `30000/1001`)    |
| `{{ .Playout.FieldOrder }}`  | `MAJMUN_PLAYOUT_FIELD_ORDER`  | playout                 | Field order (`progressive`, `tt`, `bb`, …); for deinterlace decisions |
| `{{ .Playout.AudioCodec }}`  | `MAJMUN_PLAYOUT_AUDIO_CODEC`  | playout                 | First audio stream codec (e.g. `aac`)                 |
| `{{ .Playout.AudioChannels }}` | `MAJMUN_PLAYOUT_AUDIO_CHANNELS` | playout             | First audio stream channel count                      |
| `{{ .Playout.SampleRate }}`  | `MAJMUN_PLAYOUT_SAMPLE_RATE`  | playout                 | First audio stream sample rate in Hz                  |
| `{{ .Playout.AudioLanguages }}` | `MAJMUN_PLAYOUT_AUDIO_LANGUAGES` | playout            | Space-separated language tag of each audio stream, in order (`und` when untagged), e.g. `eng rus und` |
| `{{ .Channel.Name }}`        | `MAJMUN_CHANNEL_NAME`         | playout                 | The channel's `name`                                  |
| `{{ .Channel.Logo }}`        | `MAJMUN_CHANNEL_LOGO`         | playout                 | The resolved `logo` (`""` when unset)                 |
| `{{ .Playlist.Name }}`       | `MAJMUN_PLAYLIST_NAME`        | playout                 | The parent playlist's name                            |

Environment variable names are the namespace and field flattened to upper `SNAKE_CASE` (`Stream.PlaylistPath` →
`MAJMUN_STREAM_PLAYLIST_PATH`). Empty values are not exported. User `template_variables` are **not** exported as
environment variables — use `env_variables` for that.

!!! warning "Reserved namespaces"

    `Stream`, `Playout`, `Channel` and `Playlist` are reserved and cannot be used as `template_variables` names.
    Setting one is a validation error.

## Inline or Script

Because the reserved paths are available as environment variables, a long command can live in a shell script instead of
inline YAML, which is often more readable:

=== "Inline"

    ```yaml
    command: [ffmpeg, -i, "{{ .Stream.PlaylistPath }}", -c, copy, -f, mpegts, "pipe:1"]
    ```

=== "Script"

    ```yaml
    command: [/config/scripts/stream.sh]
    ```

    ```bash
    #!/usr/bin/env bash
    exec ffmpeg -i "$MAJMUN_STREAM_PLAYLIST_PATH" -c copy -f mpegts pipe:1
    ```

Both run the same program. The inline form keeps everything in one file and supports the `template_variables` cascade;
the script form trades that cascade for plain shell variables and keeps the YAML short. A script must be executable
(`chmod +x`), readable inside the container, with its interpreter and any tools (e.g. FFmpeg) on `PATH`.

!!! warning "Use `exec` in wrapper scripts"

    End the script with `exec ffmpeg ...` (as above), so FFmpeg replaces the shell and majmun can stop it directly.
    Without `exec`, FFmpeg is left orphaned when majmun terminates the script.
