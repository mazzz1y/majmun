# Segmenter

The `segmenter` is the shared half of the [proxy pipeline](../proxy.md#how-streaming-works). For each proxied
stream, Majmun runs **one** segmenter process that fetches the upstream and slices it into HLS segments in a
temporary directory. Every viewer of that stream shares this process and its segments — the provider sees a single
connection.

Each viewer is then served by its own [stream](./stream.md) command reading from the local segments. This split is
what makes viewers independent: a slow TV can't stall a fast one, and every viewer starts cleanly on a keyframe.

Because the segmenter runs once per stream, it is the right place for expensive work like transcoding — done here,
it happens once and all viewers share the result.

## Contract

- **Input:** `{{ .url }}` — the upstream stream URL.
- **Output:** HLS segments written to `{{ .segment_path }}` and a playlist at `{{ .playlist_path }}`, continuously
  rolling (old segments deleted). Majmun waits for `init_segments` segments to appear (up to `ready_timeout`)
  before serving viewers.

## YAML Structure

```yaml
proxy:
  segmenter:
    command: []
    template_variables: []
    env_variables: []
    init_segments: 2
    ready_timeout: 30s
```

## Fields

| Field                | Type                                           | Required | Description                                                                                                              |
| -------------------- | ---------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------ |
| `command`            | [`Command`](../shared.md#command)              | No       | Segmenter command array to execute                                                                                       |
| `template_variables` | [`[]NameValue`](../shared.md#namevalue-object) | No       | Variables available in command templates                                                                                  |
| `env_variables`      | [`[]NameValue`](../shared.md#namevalue-object) | No       | Environment variables for the command                                                                                    |
| `init_segments`      | `int`                                          | No       | Number of segments that must exist before clients can start reading (default: `2`). Must be at least 1.                  |
| `ready_timeout`      | [`duration`](../shared.md#duration)            | No       | Maximum time to wait for the initial segments to become available (default: `30s`).                                       |

### Default Template Variables

These variables have default values and are used in the default segmenter command. They can be overridden via `template_variables`:

| Variable           | Default | Description                                          |
| ------------------ | ------- | ---------------------------------------------------- |
| `segment_duration` | `2`     | Duration of each HLS segment in seconds              |
| `max_segments`     | `15`    | Maximum number of segments kept in the playlist      |
| `ffmpeg_log_level` | `fatal` | FFmpeg log level                                     |

### Reserved Template Variables

These variables are injected at runtime by the system and are always available in the segmenter command templates:

| Variable        | Type     | Description                                                  |
| --------------- | -------- | ------------------------------------------------------------ |
| `url`           | `string` | Upstream stream URL                                          |
| `segment_path`  | `string` | File path for segment files (e.g. `/tmp/.../seg_%05d.ts`)    |
| `playlist_path` | `string` | File path for the HLS playlist (e.g. `/tmp/.../stream.m3u8`) |

!!! warning "Reserved Variables"

    `input`, `url`, `segment_path`, `playlist_path`, `Channel` and `Playlist` are reserved and cannot be used in
    `template_variables`. Setting them will result in a validation error.

## Examples

### Default Configuration

The default segmenter command copies the upstream stream into HLS segments without transcoding:

```yaml
proxy:
  enabled: true
  segmenter:
    command:
      - "ffmpeg"
      - "-v"
      - "{{ .ffmpeg_log_level }}"
      - "-i"
      - "{{ .url }}"
      - "-c"
      - "copy"
      - "-f"
      - "hls"
      - "-hls_time"
      - "{{ .segment_duration }}"
      - "-hls_list_size"
      - "{{ .max_segments }}"
      - "-hls_flags"
      - "delete_segments+append_list+independent_segments"
      - "-hls_segment_filename"
      - "{{ .segment_path }}"
      - "{{ .playlist_path }}"
    template_variables:
      - name: ffmpeg_log_level
        value: "fatal"
      - name: segment_duration
        value: "2"
      - name: max_segments
        value: "15"
```

### Transcoding

Transcode the stream to H.264 before segmenting. This transcodes once in the segmenter, and all clients share the transcoded segments:

```yaml
proxy:
  segmenter:
    command:
      - "ffmpeg"
      - "-v"
      - "fatal"
      - "-i"
      - "{{ .url }}"
      - "-c:v"
      - "libx264"
      - "-preset"
      - "ultrafast"
      - "-c:a"
      - "aac"
      - "-f"
      - "hls"
      - "-hls_time"
      - "{{ .segment_duration }}"
      - "-hls_list_size"
      - "{{ .max_segments }}"
      - "-hls_flags"
      - "delete_segments+append_list+independent_segments"
      - "-hls_segment_filename"
      - "{{ .segment_path }}"
      - "{{ .playlist_path }}"
```

### Low-Latency Configuration

Shorter segments and fewer init segments reduce startup latency:

```yaml
proxy:
  segmenter:
    template_variables:
      - name: segment_duration
        value: "1"
    init_segments: 1
    ready_timeout: 15s
```

### Per-Playlist Override

Override segmenter settings for a specific playlist:

```yaml
proxy:
  segmenter:
    template_variables:
      - name: segment_duration
        value: "2"
      - name: max_segments
        value: "15"

playlists:
  - name: low-bandwidth
    sources:
      - "http://example.com/playlist.m3u"
    proxy:
      segmenter:
        template_variables:
          - name: segment_duration
            value: "4"
          - name: max_segments
            value: "20"
        init_segments: 5
```
