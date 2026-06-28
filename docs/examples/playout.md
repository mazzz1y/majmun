<div style="max-width: 850px; margin: 0 auto;" markdown>

# Playout

Channels generated from local media. A `playout.command` is **required** — there is no default, because a transcode
that keeps up depends on your hardware. Start from the [Software Transcode](#software-transcode) below (or a
[hardware](#hardware-acceleration) variant) and point `command` at it.

The command lives in a shell script that reads its paths from the
`MAJMUN_*` [environment variables](../config/shared/command.md#reserved-variables) and keeps encoding settings as shell
variables at the top.

Playout runs the script **once per file** (see [How It Works](../config/playout.md#how-it-works)): it receives the
current file in `$MAJMUN_PLAYOUT_INPUT`, the seek position in `$MAJMUN_PLAYOUT_OFFSET`, and the file's video codec in
`$MAJMUN_PLAYOUT_VIDEO_CODEC`. What the command does with them is up to you. It must **append** to the shared HLS
playlist and mark a discontinuity at the join (`-hls_flags append_list+omit_endlist` plus `discont_start`).

## Basic Channels

Two generated channels from local folders — one with a logo, one shuffled. The global `command` points at the
[software transcode script](#software-transcode) below; both channels inherit it.

```yaml
server:
  listen_addr: ":8080"
  public_url: "http://localhost:8080"

url_generator:
  secret: "your-secret-key"

playout:
  command: [/config/scripts/playout.sh] # required — see Software Transcode below
  state_dir: /var/lib/majmun/state       # where channel schedules are persisted

playlists:
  - name: local
    channels:
      - name: "Cartoons 24/7"
        sources:
          - /media/cartoons
        playout:
          logo: /config/logos/cartoons.png
      - name: "Comedy Shows"
        sources:
          - /media/shows/comedy
        playout:
          order: shuffle # reshuffled when the file set changes

clients:
  - name: "tv"
    secret: "tv-secret"
    playlists: ["local"]
```

## Per-Playlist / Per-Channel Override

Settings cascade Global ➡ Playlist ➡ Channel, so a playlist sets shared defaults and a single channel overrides only
what it needs.

```yaml
playout:
  template_variables:
    - name: video_bitrate_kbps
      value: "3000"

playlists:
  - name: local
    playout:
      schedule_swap_at: "03:00" # this playlist's channels swap at 03:00
    channels:
      - name: cartoons
        sources: [/media/cartoons]
      - name: movies
        sources: [/media/movies]
        playout:
          template_variables:
            - name: video_bitrate_kbps
              value: "6000" # only this channel encodes at a higher bitrate
```

## Filler Between Episodes

A break of up to 2 minutes of clips from `/media/ads` after every 3 episodes:

```yaml
playout:
  filler:
    sources: [/media/ads]
    every_count: 3
    max_duration: 2m

playlists:
  - name: local
    channels:
      - name: series
        sources: [/media/series]
```

Or space breaks by airtime with `every` instead of `every_count`:

```yaml
playout:
  filler:
    sources: [/media/ads]
    every: 45m
    max_duration: 2m
```

## Software Transcode

A CPU (libx264) transcode with normalization, per-file seek, and a startup burst — a works-everywhere starting point.
Point `command` at the script:

```yaml
playout:
  command: [/config/scripts/playout.sh]
```

```bash title="/config/scripts/playout.sh"
#!/usr/bin/env bash
set -euo pipefail

width=1920
height=1080
fps=30
segment_duration=2
max_segments=15
initial_burst=16        # seconds produced at full speed before realtime pacing
audio_rate=48000
audio_channels=2

exec ffmpeg -v fatal \
  -re -readrate_initial_burst "$initial_burst" \
  -fflags +genpts \
  -ss "${MAJMUN_PLAYOUT_OFFSET:-0}" -i "$MAJMUN_PLAYOUT_INPUT" \
  -vf "scale=iw*sar:ih,setsar=1,\
scale=${width}:${height}:force_original_aspect_ratio=decrease,\
pad=${width}:${height}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=${fps},format=yuv420p" \
  -c:v libx264 -preset veryfast \
  -flags +cgop -sc_threshold 0 \
  -force_key_frames "expr:gte(t,n_forced*${segment_duration})" \
  -c:a aac -ar "$audio_rate" -ac "$audio_channels" \
  -f hls -hls_time "$segment_duration" -hls_list_size "$max_segments" \
  -hls_flags "delete_segments+append_list+independent_segments+omit_endlist+discont_start" \
  -hls_segment_filename "$MAJMUN_STREAM_SEGMENT_PATH" "$MAJMUN_STREAM_PLAYLIST_PATH"
```

`$MAJMUN_PLAYOUT_OFFSET`, `$MAJMUN_PLAYOUT_VIDEO_CODEC`, and the other [reserved
variables](../config/shared/command.md#reserved-variables) are yours to use — e.g. select an audio track with `-map`,
or pick a decoder from the codec. What the command does with them is entirely up to you.

`$MAJMUN_PLAYOUT_OFFSET` is non-zero only when a viewer joins partway through a programme. The examples pass it as an
input seek (`-ss` before `-i`), which is fast but can be imprecise on long-GOP or AV1 sources; for frame-accurate joins
use an output seek (`-ss` after `-i`) at the cost of decoding from the previous keyframe.

## Logo Overlay (Watermark)

Burns a per-channel logo into the stream. The resolved `logo` is exposed to the script as `$MAJMUN_CHANNEL_LOGO`.

```yaml
playout:
  command: [/config/scripts/playout-logo.sh]

playlists:
  - name: local
    channels:
      - name: cartoons
        sources: [/media/cartoons]
        playout:
          logo: /config/logos/cartoons.png
```

Relative to the default script, this adds a second input (`$MAJMUN_CHANNEL_LOGO`) and replaces `-vf` with a
`-filter_complex` that overlays it, scaled to 6% of the frame width and placed top-left with 3% margins:

```bash
#!/usr/bin/env bash
set -euo pipefail

width=1920
height=1080
fps=30
segment_duration=2
max_segments=15
initial_burst=16
audio_rate=48000
audio_channels=2

exec ffmpeg -v fatal \
  -re -readrate_initial_burst "$initial_burst" \
  -fflags +genpts \
  -ss "${MAJMUN_PLAYOUT_OFFSET:-0}" -i "$MAJMUN_PLAYOUT_INPUT" \
  -i "$MAJMUN_CHANNEL_LOGO" \
  -filter_complex "\
[0:v]scale=iw*sar:ih,setsar=1,\
scale=${width}:${height}:force_original_aspect_ratio=decrease,\
pad=${width}:${height}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=${fps}[base];\
[1:v]scale=trunc(${width}*0.06):-1[logo];\
[base][logo]overlay=x=W*0.03:y=H*0.03,format=yuv420p" \
  -c:v libx264 -preset veryfast \
  -flags +cgop -sc_threshold 0 \
  -force_key_frames "expr:gte(t,n_forced*${segment_duration})" \
  -c:a aac -ar "$audio_rate" -ac "$audio_channels" \
  -f hls -hls_time "$segment_duration" -hls_list_size "$max_segments" \
  -hls_flags "delete_segments+append_list+independent_segments+omit_endlist+discont_start" \
  -hls_segment_filename "$MAJMUN_STREAM_SEGMENT_PATH" "$MAJMUN_STREAM_PLAYLIST_PATH"
```

!!! warning "Every channel needs a logo with this command"

    `$MAJMUN_CHANNEL_LOGO` is empty for channels without a resolved `logo`, and FFmpeg fails on an empty `-i`.
    Either set `logo` for every channel served by this command, or give the script a fallback:
    `-i "${MAJMUN_CHANNEL_LOGO:-/config/logos/fallback.png}"`.

## Hardware Acceleration

Majmun does not auto-detect hardware encoders — to use one, provide a `command`. Each variant below is the default
script with only the **video filter** (`-vf`) and **encoder** (`-c:v`) changed (VAAPI also adds a device).

These are **starting points, not tested** across GPU generations and FFmpeg builds — consult the
[FFmpeg hardware acceleration docs](https://trac.ffmpeg.org/wiki/HWAccelIntro) and validate on your own setup.

=== "NVENC"

    Software-decodes and normalizes on the CPU, encodes on the GPU — the most robust split for mixed input codecs.

    ```bash
    #!/usr/bin/env bash
    set -euo pipefail

    width=1920; height=1080; fps=30
    segment_duration=2; max_segments=15; initial_burst=16
    audio_rate=48000; audio_channels=2

    exec ffmpeg -v fatal \
      -re -readrate_initial_burst "$initial_burst" \
      -fflags +genpts \
      -ss "${MAJMUN_PLAYOUT_OFFSET:-0}" -i "$MAJMUN_PLAYOUT_INPUT" \
      -vf "scale=iw*sar:ih,setsar=1,\
    scale=${width}:${height}:force_original_aspect_ratio=decrease,\
    pad=${width}:${height}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=${fps},format=yuv420p" \
      -c:v h264_nvenc \
      -force_key_frames "expr:gte(t,n_forced*${segment_duration})" \
      -c:a aac -ar "$audio_rate" -ac "$audio_channels" \
      -f hls -hls_time "$segment_duration" -hls_list_size "$max_segments" \
      -hls_flags "delete_segments+append_list+independent_segments+omit_endlist+discont_start" \
      -hls_segment_filename "$MAJMUN_STREAM_SEGMENT_PATH" "$MAJMUN_STREAM_PLAYLIST_PATH"
    ```

=== "VAAPI"

    Normalizes on the CPU, then uploads frames to a VAAPI surface for GPU encoding (`-vaapi_device` plus
    `format=nv12,hwupload` at the end of the filter).

    ```bash
    #!/usr/bin/env bash
    set -euo pipefail

    width=1920; height=1080; fps=30
    segment_duration=2; max_segments=15; initial_burst=16
    audio_rate=48000; audio_channels=2

    exec ffmpeg -v fatal -vaapi_device /dev/dri/renderD128 \
      -re -readrate_initial_burst "$initial_burst" \
      -fflags +genpts \
      -ss "${MAJMUN_PLAYOUT_OFFSET:-0}" -i "$MAJMUN_PLAYOUT_INPUT" \
      -vf "scale=iw*sar:ih,setsar=1,\
    scale=${width}:${height}:force_original_aspect_ratio=decrease,\
    pad=${width}:${height}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=${fps},format=nv12,hwupload" \
      -c:v h264_vaapi \
      -force_key_frames "expr:gte(t,n_forced*${segment_duration})" \
      -c:a aac -ar "$audio_rate" -ac "$audio_channels" \
      -f hls -hls_time "$segment_duration" -hls_list_size "$max_segments" \
      -hls_flags "delete_segments+append_list+independent_segments+omit_endlist+discont_start" \
      -hls_segment_filename "$MAJMUN_STREAM_SEGMENT_PATH" "$MAJMUN_STREAM_PLAYLIST_PATH"
    ```

=== "QSV"

    Software-decodes and normalizes on the CPU, encodes with `h264_qsv`.

    ```bash
    #!/usr/bin/env bash
    set -euo pipefail

    width=1920; height=1080; fps=30
    segment_duration=2; max_segments=15; initial_burst=16
    audio_rate=48000; audio_channels=2

    exec ffmpeg -v fatal \
      -re -readrate_initial_burst "$initial_burst" \
      -fflags +genpts \
      -ss "${MAJMUN_PLAYOUT_OFFSET:-0}" -i "$MAJMUN_PLAYOUT_INPUT" \
      -vf "scale=iw*sar:ih,setsar=1,\
    scale=${width}:${height}:force_original_aspect_ratio=decrease,\
    pad=${width}:${height}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=${fps},format=yuv420p" \
      -c:v h264_qsv \
      -force_key_frames "expr:gte(t,n_forced*${segment_duration})" \
      -c:a aac -ar "$audio_rate" -ac "$audio_channels" \
      -f hls -hls_time "$segment_duration" -hls_list_size "$max_segments" \
      -hls_flags "delete_segments+append_list+independent_segments+omit_endlist+discont_start" \
      -hls_segment_filename "$MAJMUN_STREAM_SEGMENT_PATH" "$MAJMUN_STREAM_PLAYLIST_PATH"
    ```

See also: [Playout](../config/playout.md), [Channels](../config/channels.md), [Command](../config/shared/command.md).

</div>
