# Stream

The `stream` block configures the command executed for stream processing during proxy/remuxing.
This command is used to process video streams, optionally applying transcoding, filtering, or other transformations.

!!! note "Command Execution"

    Majmun expects the command to output video stream data to `stdout`. `stderr` will be printed to the debug logs.
    If the command exits with empty stdout, an upstream error will be triggered.

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
