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

| Field                | Type                               | Required | Description                              |
|----------------------|------------------------------------|----------|------------------------------------------|
| `command`            | `gotemplate` or `[]gotemplate`     | No       | Command array to execute                 |
| `template_variables` | [`[]NameValue`](#namevalue-object) | No       | Variables available in command templates |
| `env_variables`      | [`[]NameValue`](#namevalue-object) | No       | Environment variables for the command    |

### Name/Value Object

| Field   | Type     | Required | Description                          |
|---------|----------|----------|--------------------------------------|
| `name`  | `string` | Yes      | Name identifier for the object       |
| `value` | `string` | Yes      | Value associated with the given name |

### Available Template Variables

| Variable | Type     | Description |
|----------|----------|-------------|
| `url`    | `string` | Stream URL  |

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
      - "{{ .url }}"
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
      - "{{ default \"fatal\" .ffmpeg_log_level }}"
      - "-i"
      - "{{ .url }}"
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

```yaml
proxy:
  stream:
    command:
      - "ffmpeg"
      - "-v"
      - "error"
      - "-i"
      - "{{ .url }}"
      - "-c"
      - "copy"
      - "-f"
      - "mpegts"
      - "pipe:1"
    env_variables:
      - name: FFMPEG_BINARY
        value: "/usr/bin/ffmpeg"
      - name: HWACCEL
        value: "auto"
```
