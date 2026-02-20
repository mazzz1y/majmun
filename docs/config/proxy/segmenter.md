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
    segment_duration: 2
    max_segments: 15
    init_segments: 2
    ready_timeout: 30s
```

## Fields

| Field                | Type                               | Required | Description                                                              |
|----------------------|------------------------------------|----------|--------------------------------------------------------------------------|
| `command`            | [`Command`](../shared.md#command)          | No       | Segmenter command array to execute                                       |
| `template_variables` | [`[]NameValue`](../shared.md#namevalue-object) | No       | Variables available in command templates                                 |
| `env_variables`      | [`[]NameValue`](../shared.md#namevalue-object) | No       | Environment variables for the command                                    |
| `segment_duration`   | `int`                                     | No       | Duration of each HLS segment in seconds (default: `2`). Must be at least 1.            |
| `max_segments`       | `int`                                     | No       | Maximum number of segments kept in the playlist (default: `15`). Must be at least 1.    |
| `init_segments`      | `int`                                     | No       | Number of segments that must exist before clients can start reading (default: `2`). Must be at least 1 and cannot exceed `max_segments`. |
| `ready_timeout`      | [`duration`](../shared.md#duration)       | No       | Maximum time to wait for the initial segments to become available (default: `30s`).     |

### Available Template Variables

These variables are automatically injected by the system and are always available in the segmenter command templates:

| Variable           | Type     | Description                                                       |
|--------------------|----------|-------------------------------------------------------------------|
| `segment_duration` | `string` | Value of `segment_duration` config field                          |
| `max_segments`     | `string` | Value of `max_segments` config field                              |
| `segment_path`     | `string` | File path for segment files (e.g. `/tmp/.../seg_%05d.ts`)         |
| `playlist_path`    | `string` | File path for the HLS playlist (e.g. `/tmp/.../stream.m3u8`)     |

!!! warning "Reserved Variables"

    These variable names are reserved and cannot be used in `template_variables`. Setting them will result in a validation error.

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
      - "{{ default \"fatal\" .ffmpeg_log_level }}"
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
    segment_duration: 1
    init_segments: 1
    ready_timeout: 15s
```

### Per-Playlist Override

Override segmenter settings for a specific playlist:

```yaml
proxy:
  segmenter:
    segment_duration: 2
    max_segments: 15

playlists:
  - name: low-bandwidth
    sources:
      - url: "http://example.com/playlist.m3u"
    proxy:
      segmenter:
        segment_duration: 4
        max_segments: 20
        init_segments: 5 # 4s * 5 = 20s buffered before clients can start
```
