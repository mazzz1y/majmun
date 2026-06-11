# Stream

The `stream` command is the per-viewer half of the [proxy pipeline](../proxy.md#how-streaming-works): it runs once
for every connected TV, reads the local segments produced by the shared [segmenter](./segmenter.md) (or
[playout](./playout.md)) process, and sends the result to that viewer. That's why its input is a local file path,
not the upstream URL — the upstream is handled by the segmenter.

Because it runs per viewer, this is the place for cheap per-viewer processing. The default just repackages the
video without re-encoding; transcoding here would run once per TV, so heavy work belongs in the segmenter or
playout command instead.

## Contract

- **Input:** `{{ .input }}` — path to the local HLS playlist written by the segmenter/playout process.
- **Output:** video stream data on `stdout`. `stderr` is printed to the debug logs. If the command exits with empty
  stdout, an upstream error is triggered.

## YAML Structure

```yaml
proxy:
  stream:
    command: []
    template_variables: []
    env_variables: []
```

## Fields

| Field                | Type                                           | Required | Description                              |
| -------------------- | ---------------------------------------------- | -------- | ---------------------------------------- |
| `command`            | [`Command`](../shared.md#command)              | No       | Command array to execute                 |
| `template_variables` | [`[]NameValue`](../shared.md#namevalue-object) | No       | Variables available in command templates |
| `env_variables`      | [`[]NameValue`](../shared.md#namevalue-object) | No       | Environment variables for the command    |

### Reserved Template Variables

These variables are injected at runtime by the system and are always available in the stream command templates:

| Variable | Type     | Description                                              |
| -------- | -------- | -------------------------------------------------------- |
| `input`  | `string` | Path to the local HLS playlist produced by the segmenter |

### Default Template Variables

These variables are used by the default command and can be overridden:

| Variable           | Default | Description           |
| ------------------ | ------- | --------------------- |
| `ffmpeg_log_level` | `fatal` | ffmpeg `-v` log level |

## Examples

### Basic FFmpeg Remuxing

```yaml
proxy:
  stream:
    command:
      - "ffmpeg"
      - "-v"
      - "error"
      - "-i"
      - "{{ .input }}"
      - "-c"
      - "copy"
      - "-f"
      - "mpegts"
      - "pipe:1"
```

### Transcoding with Custom Preset

```yaml
proxy:
  stream:
    command:
      - "ffmpeg"
      - "-v"
      - "{{ .ffmpeg_log_level }}"
      - "-i"
      - "{{ .input }}"
      - "-c:v"
      - "libx264"
      - "-preset"
      - "ultrafast"
      - "-c:a"
      - "aac"
      - "-f"
      - "mpegts"
      - "pipe:1"
    template_variables:
      - name: ffmpeg_log_level
        value: "error"
```

### With Environment Variables

Environment variables are passed to the command process. This is useful for commands that read configuration from the environment.

```yaml
proxy:
  stream:
    command:
      - "/opt/scripts/stream.sh"
      - "{{ .input }}"
    env_variables:
      - name: STREAM_QUALITY
        value: "high"
      - name: LOG_DIR
        value: "/var/log/streams"
```
