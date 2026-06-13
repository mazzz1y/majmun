# Proxy

The proxy block makes Majmun sit between your TVs and the upstream streams. With proxying disabled, playlists
contain the provider's original stream URLs and TVs connect to the provider directly. With proxying enabled, the
URLs are rewritten to encrypted links pointing at your `public_url`, and the video flows through Majmun.

The default configuration uses FFmpeg and works out of the box — most users only need `enabled: true`. The commands
are fully configurable for transcoding, filtering, or any other processing.

## How Streaming Works

When a TV requests a proxied stream, two commands cooperate:

1. The [**segmenter**](proxy/segmenter.md) fetches the upstream stream **once** and slices it into small video
   segments in a temporary directory. All viewers of the same stream share this one process — the provider sees a
   single connection no matter how many TVs are watching.
2. The [**stream**](proxy/stream.md) command runs **once per viewer**, reads the shared segments, and sends the
   result to that TV. By default it just repackages the video without re-encoding (often called "remuxing"), which
   is cheap.

[Generated channels](channels.md) replace step 1 with the [**playout**](playout.md) command, which produces a
continuous stream from your local media files instead of fetching an upstream. Playout is configured in its own
top-level [`playout`](playout.md) block, not under `proxy`. When anything goes wrong — upstream down, limit reached,
expired link — an [**error**](proxy/error.md) command generates a short video telling the viewer what happened.

!!! warning "Shared transcodes: first viewer wins"

    Because segmenter and playout processes are shared, they start with the config of whichever viewer
    requests the stream first — everyone else joins that stream as-is. In practice: **per-client `segmenter`
    overrides don't isolate clients.** Keep command differences at the global or playlist level. (Playout has no
    per-client level at all — see [Playout](playout.md).)

## Configuration Levels

Proxy can be defined globally, per playlist, and per client. Each level overrides the previous one, field by field:

Global Proxy ➡ Playlist Proxy ➡ Client Proxy

The exception is `concurrency`: rather than overriding, each level enforces its own limit independently. A stream
starts only if the global, playlist, and client limits all have capacity, and exceeding any of them triggers the
`rate_limit_exceeded` [error handler](proxy/error.md).

!!! note "Command Output"

    The `stream` and `error` commands must write video data to `stdout`; `stderr` is printed to the debug logs.
    If a command exits with empty stdout, an upstream error is triggered. The `segmenter` command (and the
    [playout](playout.md) transcode for generated channels) write segment files to disk instead — see their pages
    for the exact contract.

## YAML Structure

```yaml
proxy:
  enabled: false
  concurrency: 0
  http_client:
    cache:
      enabled: true
      path: ""
      ttl: ""
      retention: ""
      compression: false
    headers: []
  stream:
    command: []
    template_variables: []
    env_variables: []
  segmenter:
    command: []
    template_variables: []
    env_variables: []
    init_segments: 2
    ready_timeout: 30s
  error:
    command: []
    template_variables: []
    env_variables: []
    upstream_error:
      command: []
      template_variables: []
      env_variables: []
    rate_limit_exceeded:
      command: []
      template_variables: []
      env_variables: []
    link_expired:
      command: []
      template_variables: []
      env_variables: []
```

## Fields

### Main Proxy Configuration

| Field         | Type                                   | Required | Description                                        |
| ------------- | -------------------------------------- | -------- | -------------------------------------------------- |
| `enabled`     | `bool`                                 | No       | Enable or disable proxy functionality              |
| `concurrency` | `int`                                  | No       | Maximum concurrent streams (0 = unlimited)         |
| `http_client` | [`HTTPClient`](./proxy/http_client.md) | No       | HTTP client configuration overrides for this proxy |
| `stream`      | [`Stream`](./proxy/stream.md)          | No       | Command configuration for stream processing        |
| `segmenter`   | [`Segmenter`](./proxy/segmenter.md)    | No       | HLS segmenter for remote proxied streams           |
| `error`       | [`Error`](./proxy/error.md)            | No       | Default error handling configuration               |

Generated channels are configured separately in the top-level [`playout`](playout.md) block, not under `proxy`.

### Related Documentation

- [Stream Processing](./proxy/stream.md) - Configure stream remuxing commands
- [Segmenter](./proxy/segmenter.md) - Configure HLS segmenter for proxied streams
- [Playout](./playout.md) - Configure generated channels (transcode + scheduling)
- [Error Handling](./proxy/error.md) - Configure error fallback content
- [HTTP Client](./proxy/http_client.md) - Configure HTTP request settings

## Examples

### Basic Proxy Setup

```yaml
proxy:
  enabled: true
  concurrency: 10
```

### Custom FFmpeg Configuration

```yaml
proxy:
  enabled: true
  concurrency: 5
  stream:
    command:
      [ffmpeg, -v, "{{ .ffmpeg_log_level }}", -i, "{{ .Stream.PlaylistPath }}",
       -c:v, libx264, -preset, ultrafast, -f, mpegts, "pipe:1"]
    template_variables:
      - { name: ffmpeg_log_level, value: error }
```

### Proxy HTTP Client Overrides

```yaml
proxy:
  enabled: true
  http_client:
    cache:
      enabled: true
      ttl: 5m
    headers:
      - name: User-Agent
        value: "MyUA"
```
