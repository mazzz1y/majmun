# Segmenter

The `segmenter` block configures the HLS segmenter used for stream pooling. For each proxied stream, Majmun runs a single upstream fetch and an FFmpeg HLS segmenter that writes segments to a temporary directory. Each client is served by an independent FFmpeg process reading from the local HLS playlist, ensuring keyframe-aligned starts and isolation between slow and fast clients. When multiple clients request the same stream, they share the same segmenter and segment directory.

The segmenter command receives the upstream stream data on `stdin` and must output HLS segments to the paths provided via template variables.

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
| `segment_path`  | `string` | File path for segment files (e.g. `/tmp/.../seg_%05d.ts`)    |
| `playlist_path` | `string` | File path for the HLS playlist (e.g. `/tmp/.../stream.m3u8`) |

!!! warning "Reserved Variables"

    `segment_path` and `playlist_path` are reserved and cannot be used in `template_variables`. Setting them will result in a validation error.

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
      - '{{ default "fatal" .ffmpeg_log_level }}'
      - "-i"
      - "pipe:0"
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
      - "pipe:0"
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
      - url: "http://example.com/playlist.m3u"
    proxy:
      segmenter:
        template_variables:
          - name: segment_duration
            value: "4"
          - name: max_segments
            value: "20"
        init_segments: 5
```
