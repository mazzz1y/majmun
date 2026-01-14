# Proxy

The proxy block configures the streaming proxy functionality, also known as "remuxing".
This feature allows the app to act as an intermediary between IPTV clients and upstream sources, providing stream
processing, transcoding, and error handling capabilities.

When proxying is enabled, the links in the playlist will be encrypted and will point to the `public_url`.

The default configuration uses FFmpeg for remuxing and is ready to use out of the box. Most users can enable proxy
functionality by simply setting `enabled` to `true`. Advanced users can customize commands to add transcoding,
filtering, or other stream processing features.

!!! note "Rule Merging Order"

    Proxy can be defined at multiple levels in the configuration. It will be merged in the following order, with each level overriding the previous one:

    Global Proxy ➡ Subscription Proxy ➡ Client Proxy

    This applies to all proxy-related fields, **except concurrency**.

!!! note "Concurrency Handling"

    Concurrency is handled at the global, subscription, and client levels separately.

!!! note "Command Handling"

    Majmun expects the command to output video stream data to `stdout`. `stderr` will be printed to the debug logs.
    If the command exits with empty stdout, an upstream error will be triggered.

## YAML Structure

```yaml
proxy:
  enabled: false
  concurrency: 0
  http_client:
    cache:
      enabled: true
      ttl: ""
      retention: ""
      compression: false
    headers: []
  stream:
    command: []
    template_variables: []
    env_variables: []
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
|---------------|----------------------------------------|----------|----------------------------------------------------|
| `enabled`     | `bool`                                 | No       | Enable or disable proxy functionality              |
| `concurrency` | `int`                                  | No       | Maximum concurrent streams (0 = unlimited)         |
| `http_client` | [`HTTPClient`](./proxy/http_client.md) | No       | HTTP client configuration overrides for this proxy |
| `stream`      | `command`                              | No       | Command configuration for stream processing        |
| `error`       | `command`                              | No       | Default error handling configuration               |

### Related Documentation

- [Stream Processing](./proxy/stream.md) - Configure stream remuxing commands
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
      - "ffmpeg"
      - "-v"
      - "{{ default \"fatal\" .ffmpeg_log_level }}"
      - "-i"
      - "{{ .url }}"
      - "-c:v"
      - "libx264"
      - "-preset"
      - "ultrafast"
      - "-f"
      - "mpegts"
      - "pipe:1"
    template_variables:
      - name: ffmpeg_log_level
        value: "error"
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
