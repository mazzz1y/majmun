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

| Field         | Type                             | Required | Description                                        |
|---------------|----------------------------------|----------|----------------------------------------------------|
| `enabled`     | `bool`                           | No       | Enable or disable proxy functionality              |
| `concurrency` | `int`                            | No       | Maximum concurrent streams (0 = unlimited)         |
| `http_client` | [`HTTPClient`](./http_client.md) | No       | HTTP client configuration overrides for this proxy |
| `stream`      | `command`                        | No       | Command configuration for stream processing        |
| `error`       | `command`                        | No       | Default error handling configuration               |

### Command Object

!!! note "Command String Format"
Command can be specified as a string or an array of strings, similar to Dockerfile syntax. If the command is specified
as a string, it will be wrapped in a `/bin/sh` shell.

| Field                | Type                               | Required | Description                              |
|----------------------|------------------------------------|----------|------------------------------------------|
| `command`            | `gotemplate` or `[]gotemplate`     | No       | Command array to execute                 |
| `template_variables` | [`[]NameValue`](#namevalue-object) | No       | Variables available in command templates |
| `env_variables`      | [`[]NameValue`](#namevalue-object) | No       | Environment variables for the command    |

### Error Handling Objects

| Field                 | Type      | Required | Description                               |
|-----------------------|-----------|----------|-------------------------------------------|
| `upstream_error`      | `command` | No       | Command to run when upstream source fails |
| `rate_limit_exceeded` | `command` | No       | Command to run when rate limits are hit   |
| `link_expired`        | `command` | No       | Command to run when stream links expire   |

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

### Error Handling with Test Pattern

```yaml
proxy:
  enabled: true
  error:
    upstream_error:
      command:
        - "ffmpeg"
        - "-v"
        - "{{ default \"fatal\" .ffmpeg_log_level }}"
        - "-f"
        - "lavfi"
        - "-i"
        - "testsrc2=size=1280x720:rate=25"
        - "-f"
        - "lavfi"
        - "-i"
        - "sine=frequency=1000:duration=0"
        - "-c:v"
        - "libx264"
        - "-c:a"
        - "aac"
        - "-t"
        - "3600"
        - "-f"
        - "mpegts"
        - "pipe:1"
      template_variables:
        - name: ffmpeg_log_level
          value: "fatal"
    rate_limit_exceeded:
      template_variables:
        - name: message
          value: "Rate limit exceeded. Please try again later."
    link_expired:
      template_variables:
        - name: message
          value: "Link has expired. Please refresh your playlist."
```
