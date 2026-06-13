# Stream

The `stream` command is the per-viewer half of the [proxy pipeline](../proxy.md#how-streaming-works): it runs once
for every connected TV, reads the local segments produced by the shared [segmenter](./segmenter.md) (or
[playout](../playout.md)) process, and sends the result to that viewer. That's why its input is a local file path,
not the upstream URL — the upstream is handled by the segmenter.

Because it runs per viewer, this is the place for cheap per-viewer processing. The default just repackages the
video without re-encoding; transcoding here would run once per TV, so heavy work belongs in the segmenter or
playout command instead.

## Contract

- **Input:** `{{ .Stream.PlaylistPath }}` — path to the local HLS playlist written by the segmenter/playout process.
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

| Field                | Type                                           | Required | Description                                                                                |
| -------------------- | ---------------------------------------------- | -------- | ------------------------------------------------------------------------------------------ |
| `command`            | [`Command`](../shared/command.md)                     | No       | Stream command. See [Command](../shared/command.md).                                              |
| `template_variables` | [`[]NameValue`](../shared/name-value.md) | No       | User variables available in the command. See [Command](../shared/command.md#template-variables).  |
| `env_variables`      | [`[]NameValue`](../shared/name-value.md) | No       | Environment variables for the command.                                                     |

The stream command receives the reserved variable `{{ .Stream.PlaylistPath }}` (the local HLS playlist) — see
[Command → Reserved Variables](../shared/command.md#reserved-variables).

### Default Template Variables

These variables are used by the default command and can be overridden:

| Variable           | Default | Description           |
| ------------------ | ------- | --------------------- |
| `ffmpeg_log_level` | `fatal` | ffmpeg `-v` log level |

## Examples

### Basic FFmpeg Remuxing

Repackage the local segments without re-encoding (the default behavior):

```yaml
proxy:
  stream:
    command:
      [ffmpeg, -v, error, -i, "{{ .Stream.PlaylistPath }}", -c, copy, -f, mpegts, "pipe:1"]
```

### Transcoding with Custom Preset

Re-encode per viewer (heavier — prefer transcoding in the [segmenter](./segmenter.md) instead):

```yaml
proxy:
  stream:
    command:
      [ffmpeg, -v, "{{ .ffmpeg_log_level }}", -i, "{{ .Stream.PlaylistPath }}",
       -c:v, libx264, -preset, ultrafast, -c:a, aac, -f, mpegts, "pipe:1"]
    template_variables:
      - { name: ffmpeg_log_level, value: error }
```

### With Environment Variables

Pass configuration to a wrapper script via the environment:

```yaml
proxy:
  stream:
    command: [/opt/scripts/stream.sh, "{{ .Stream.PlaylistPath }}"]
    env_variables:
      - { name: STREAM_QUALITY, value: high }
      - { name: LOG_DIR, value: /var/log/streams }
```
