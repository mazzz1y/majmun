# Playout

The `playout` command turns a generated [channel](../channels.md)'s media files into a continuous live stream. It
takes the channel's schedule as input and produces rolling HLS segments that viewers are served from, looping the
schedule forever.

In the [proxy pipeline](../proxy.md#how-streaming-works), playout plays the same role for generated channels that
the [segmenter](./segmenter.md) plays for proxied streams — the shared, once-per-stream half. The difference is the
input: the segmenter reads a remote upstream via `{{ .url }}`, while playout reads a local FFmpeg concat list via
`{{ .input }}`.

There is one playout transcode per channel, [shared across all viewers](../proxy.md) — first viewer's config wins.
Like all proxy blocks, `playout` cascades **Global ➡ Playlist ➡ Client**, so a playlist can override the channel
command for just its own channels.

## How It Works

For each requested channel, majmun resolves the current playback position from the persisted schedule, writes an
FFmpeg **concat list** into a scratch directory, and launches the playout command. The command runs continuously
(one process per channel, regardless of viewer count) until the last viewer disconnects. When the schedule changes
(files added/removed), a fresh process is started with a new concat list while the old one drains.

### Input

`{{ .input }}` is the path to a [concat demuxer](https://ffmpeg.org/ffmpeg-formats.html#concat-1) list containing the
full schedule, rotated so the currently-airing file is first, with an `inpoint` line seeking into it at the live
position. The files are your raw media — mixed containers, codecs, resolutions, frame rates, and audio layouts are all
possible inputs the command must cope with.

### Output

The command must produce a **live HLS stream** on disk:

- segments written to `{{ .segment_path }}` (a pattern like `/tmp/.../seg_%05d.ts`),
- a playlist at `{{ .playlist_path }}` (`/tmp/.../stream.m3u8`) that stays live (no `EXT-X-ENDLIST`) and rolls
  (old segments deleted),
- paced at **realtime** — the process loops forever, so without pacing it would transcode the whole library at full
  speed and fill the disk.

majmun waits for `init_segments` segments to appear (up to `ready_timeout`), then serves the playlist to viewers.
Anything that satisfies this contract works; the flags that achieve it are FFmpeg's domain — see the
[FFmpeg documentation](https://ffmpeg.org/ffmpeg-all.html) for what each option does.

### What a command must take care of

- **Normalization.** A continuous stream needs one uniform output. Sources with different resolutions, frame rates, or
  audio layouts must be converted to a common format, or the stream breaks at the first differing file. The default
  command scales/letterboxes everything onto a `width`×`height` canvas at `fps` and normalizes audio; if you write
  your own command without a scaling filter, all sources must already be uniform.
- **Looping.** The schedule must repeat forever (`-stream_loop -1` in the default).
- **Segment alignment.** Keyframes should land on segment boundaries so joins are fast and segments uniform.
- **Buffering.** Realtime pacing alone gives the player only ~one segment of headroom, so any encoder hiccup
  stalls playback. The default uses `-readrate_initial_burst` to produce the first `initial_burst` seconds at full
  speed, building a buffer in front of the player without delaying start. See the
  [Startup buffer](#default-software-transcode) note below for how this interacts with `init_segments`.

## YAML Structure

```yaml
proxy:
  playout:
    command: []
    template_variables: []
    env_variables: []
    init_segments: 4
    ready_timeout: 30s
```

## Fields

| Field                | Type                                           | Required | Description                                                                                             |
| -------------------- | ---------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------------- |
| `command`            | [`Command`](../shared.md#command)              | No       | Transcoder command array. Defaults to a software (libx264/aac) 24/7 loop.                               |
| `template_variables` | [`[]NameValue`](../shared.md#namevalue-object) | No       | Variables available in command templates                                                                |
| `env_variables`      | [`[]NameValue`](../shared.md#namevalue-object) | No       | Environment variables for the command                                                                   |
| `init_segments`      | `int`                                          | No       | Number of segments that must exist before clients can start reading (default: `4`). Must be at least 1. |
| `ready_timeout`      | [`duration`](../shared.md#duration)            | No       | Maximum time to wait for the initial segments to become available (default: `30s`).                     |

### Default Template Variables

These have defaults and can be overridden via `template_variables`:

| Variable           | Default | Description                                                                       |
| ------------------ | ------- | --------------------------------------------------------------------------------- |
| `ffmpeg_log_level` | `fatal` | FFmpeg log level                                                                  |
| `segment_duration` | `2`     | Duration of each HLS segment in seconds                                           |
| `max_segments`     | `15`    | Maximum number of segments kept in the playlist                                   |
| `initial_burst`    | `16`    | Seconds of output produced at full speed before realtime pacing kicks in (buffer) |
| `width`            | `1920`  | Output width every source is scaled/padded to                                     |
| `height`           | `1080`  | Output height every source is scaled/padded to                                    |
| `fps`              | `30`    | Output frame rate every source is converted to                                    |
| `audio_rate`       | `48000` | Output audio sample rate (Hz)                                                     |
| `audio_channels`   | `2`     | Output audio channel count                                                        |

### Reserved Template Variables

These are injected at runtime by the system and are always available in the playout command:

| Variable          | Type     | Description                                                       |
| ----------------- | -------- | ----------------------------------------------------------------- |
| `input`           | `string` | Path to the FFmpeg concat list (full schedule, rotated + seeked)  |
| `segment_path`    | `string` | File path for segment files (e.g. `/tmp/.../seg_%05d.ts`)         |
| `playlist_path`   | `string` | File path for the HLS playlist (e.g. `/tmp/.../stream.m3u8`)      |
| `Channel.Name`    | `string` | The channel's `name`                                              |
| `Channel.Logo`    | `string` | The channel's `logo` value (`""` when unset)                      |
| `Playlist.Name`   | `string` | The parent playlist's name                                        |

`Channel`/`Playlist` use the same nested form as [`fields`](../channels.md) templates, so one global command can
adapt per channel — e.g. `-i "{{ .Channel.Logo }}"` for a per-channel watermark. Channel-level
[`template_variables`](../channels.md) cover anything custom.

!!! warning "Reserved Variables"

    `input`, `url`, `segment_path`, `playlist_path`, `Channel` and `Playlist` are reserved and cannot be used in
    `template_variables`. Setting them will result in a validation error.

## Examples

### Default Software Transcode

The built-in command, shown explicitly. This is used when `command` is omitted:

```yaml
proxy:
  playout:
    command:
      [ffmpeg, -v, "{{ .ffmpeg_log_level }}", -re, -readrate_initial_burst, "{{ .initial_burst }}",
       -fflags, "+genpts", -stream_loop, "-1",
       -f, concat, -safe, "0", -i, "{{ .input }}",
       -vf, "scale=iw*sar:ih,setsar=1,scale={{ .width }}:{{ .height }}:force_original_aspect_ratio=decrease,pad={{ .width }}:{{ .height }}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps={{ .fps }},format=yuv420p",
       -c:v, libx264, -preset, veryfast,
       -flags, "+cgop", -sc_threshold, "0",
       -force_key_frames, "expr:gte(t,n_forced*{{ .segment_duration }})",
       -c:a, aac, -ar, "{{ .audio_rate }}", -ac, "{{ .audio_channels }}",
       -f, hls, -hls_time, "{{ .segment_duration }}", -hls_list_size, "{{ .max_segments }}",
       -hls_flags, "delete_segments+append_list+independent_segments+omit_endlist",
       -hls_segment_filename, "{{ .segment_path }}", "{{ .playlist_path }}"]
```

!!! warning "The default is software transcoding — it will be slow"

    The default command encodes with libx264 on the CPU. A single 1080p30 channel can saturate several cores, and it
    will be too slow on low-power hardware. It is a safe, works-everywhere baseline — not a recommendation. Choosing a
    command that fits your hardware (hardware encoder, lower resolution, different preset) is **your responsibility**;
    majmun runs whatever you give it and does not validate that it can keep up.

!!! note "Startup buffer"

    Playback starts once `init_segments` segments exist (default `4`, ~8s with 2s segments). Requiring several segments
    up front anchors the player behind the live edge, so it keeps a standing buffer instead of draining to zero on
    every encoder hiccup. Thanks to `initial_burst`, those first segments are produced at full speed, so the wait is
    short. Lower `init_segments` for faster joins at the cost of buffer; raise it (and `initial_burst`) for more
    resilience on slow encoders.

### Per-Playlist Override

A playlist can override the playout command for just its own channels via the proxy cascade:

```yaml
proxy:
  playout:
    template_variables:
      - name: segment_duration
        value: "2"

playlists:
  - name: local
    channels:
      - name: cartoons
        sources: [/media/cartoons]
    proxy:
      playout:
        template_variables:
          - name: segment_duration
            value: "4"
```

### Logo Overlay (Watermark)

A per-channel logo can be burned into the stream by overlaying `{{ .Channel.Logo }}` onto the normalized canvas.
This example scales the logo to 6% of the frame width and places it top-left with 3% margins:

```yaml
proxy:
  playout:
    command:
      [ffmpeg, -v, "{{ .ffmpeg_log_level }}", -re, -readrate_initial_burst, "{{ .initial_burst }}",
       -fflags, "+genpts", -stream_loop, "-1",
       -f, concat, -safe, "0", -i, "{{ .input }}",
       -i, "{{ .Channel.Logo }}",
       -filter_complex, "[0:v]scale=iw*sar:ih,setsar=1,scale={{ .width }}:{{ .height }}:force_original_aspect_ratio=decrease,pad={{ .width }}:{{ .height }}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps={{ .fps }}[base];[1:v]scale=trunc({{ .width }}*0.06):-1[logo];[base][logo]overlay=x=W*0.03:y=H*0.03,format=yuv420p",
       -c:v, libx264, -preset, veryfast,
       -flags, "+cgop", -sc_threshold, "0",
       -force_key_frames, "expr:gte(t,n_forced*{{ .segment_duration }})",
       -c:a, aac, -ar, "{{ .audio_rate }}", -ac, "{{ .audio_channels }}",
       -f, hls, -hls_time, "{{ .segment_duration }}", -hls_list_size, "{{ .max_segments }}",
       -hls_flags, "delete_segments+append_list+independent_segments+omit_endlist",
       -hls_segment_filename, "{{ .segment_path }}", "{{ .playlist_path }}"]

playlists:
  - name: local
    channels:
      - name: cartoons
        logo: /config/logos/cartoons.png
        sources: [/media/cartoons]
```

The logo image is input `[1:v]`; PNG transparency is respected, and overlay's default `eof_action=repeat` keeps the
still frame for the whole stream. The watermark is burned in — every viewer sees it.

!!! warning "Every channel needs a logo with this command"

    `{{ .Channel.Logo }}` renders empty for channels without `logo`, and FFmpeg fails on an empty `-i`. Either set
    `logo` on every channel served by this command, or use a Sprig default:
    `-i "{{ .Channel.Logo | default \"/config/logos/fallback.png\" }}"`.

### Hardware Acceleration

!!! warning "Hardware acceleration is explicit"

    Majmun does not auto-detect hardware encoders or decide between copy and transcode. To use NVENC, VAAPI, QSV, or
    stream copy, provide a `command`.

!!! warning "Untested examples — contributions welcome"

    The commands below are starting points, **not tested** on real hardware across driver/FFmpeg versions. Encoder
    options vary by GPU generation and build — consult the
    [FFmpeg hardware acceleration docs](https://trac.ffmpeg.org/wiki/HWAccelIntro) and validate on your own setup.
    If you get one working (or fix one), contributions are welcome.

=== "NVENC"

    ```yaml
    proxy:
      playout:
        command:
          [ffmpeg, -v, "{{ .ffmpeg_log_level }}", -re, -readrate_initial_burst, "{{ .initial_burst }}",
           -fflags, "+genpts", -stream_loop, "-1",
           -f, concat, -safe, "0", -i, "{{ .input }}",
           -vf, "scale=iw*sar:ih,setsar=1,scale={{ .width }}:{{ .height }}:force_original_aspect_ratio=decrease,pad={{ .width }}:{{ .height }}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps={{ .fps }},format=yuv420p",
           -c:v, h264_nvenc,
           -force_key_frames, "expr:gte(t,n_forced*{{ .segment_duration }})",
           -c:a, aac, -ar, "{{ .audio_rate }}", -ac, "{{ .audio_channels }}",
           -f, hls, -hls_time, "{{ .segment_duration }}", -hls_list_size, "{{ .max_segments }}",
           -hls_flags, "delete_segments+append_list+independent_segments+omit_endlist",
           -hls_segment_filename, "{{ .segment_path }}", "{{ .playlist_path }}"]
    ```

    Software-decodes and normalizes on the CPU, encodes on the GPU — the most robust split for mixed input codecs.

=== "VAAPI"

    ```yaml
    proxy:
      playout:
        command:
          [ffmpeg, -v, "{{ .ffmpeg_log_level }}", -vaapi_device, /dev/dri/renderD128,
           -re, -readrate_initial_burst, "{{ .initial_burst }}",
           -fflags, "+genpts", -stream_loop, "-1",
           -f, concat, -safe, "0", -i, "{{ .input }}",
           -vf, "scale=iw*sar:ih,setsar=1,scale={{ .width }}:{{ .height }}:force_original_aspect_ratio=decrease,pad={{ .width }}:{{ .height }}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps={{ .fps }},format=nv12,hwupload",
           -c:v, h264_vaapi,
           -force_key_frames, "expr:gte(t,n_forced*{{ .segment_duration }})",
           -c:a, aac, -ar, "{{ .audio_rate }}", -ac, "{{ .audio_channels }}",
           -f, hls, -hls_time, "{{ .segment_duration }}", -hls_list_size, "{{ .max_segments }}",
           -hls_flags, "delete_segments+append_list+independent_segments+omit_endlist",
           -hls_segment_filename, "{{ .segment_path }}", "{{ .playlist_path }}"]
    ```

    Normalizes on the CPU, then uploads frames to VAAPI surfaces for GPU encoding.

=== "QSV"

    ```yaml
    proxy:
      playout:
        command:
          [ffmpeg, -v, "{{ .ffmpeg_log_level }}", -re, -readrate_initial_burst, "{{ .initial_burst }}",
           -fflags, "+genpts", -stream_loop, "-1",
           -f, concat, -safe, "0", -i, "{{ .input }}",
           -vf, "scale=iw*sar:ih,setsar=1,scale={{ .width }}:{{ .height }}:force_original_aspect_ratio=decrease,pad={{ .width }}:{{ .height }}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps={{ .fps }},format=yuv420p",
           -c:v, h264_qsv,
           -force_key_frames, "expr:gte(t,n_forced*{{ .segment_duration }})",
           -c:a, aac, -ar, "{{ .audio_rate }}", -ac, "{{ .audio_channels }}",
           -f, hls, -hls_time, "{{ .segment_duration }}", -hls_list_size, "{{ .max_segments }}",
           -hls_flags, "delete_segments+append_list+independent_segments+omit_endlist",
           -hls_segment_filename, "{{ .segment_path }}", "{{ .playlist_path }}"]
    ```

    Software-decodes and normalizes on the CPU, encodes with `h264_qsv`. Full GPU decode (`-hwaccel qsv`) is picky
    about input codecs and can stall on mixed libraries — prefer the software-decode split above.
