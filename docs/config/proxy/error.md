# Error

The `error` block configures error handling commands executed during proxy/remuxing when specific error conditions
occur.
These commands allow you to display custom error content such as test patterns, audio messages, or static images
when stream errors are encountered.

!!! note "Error Types"

    Majmun supports three types of error handling:
    - `upstream_error` - Triggered when the upstream source fails
    - `rate_limit_exceeded` - Triggered when rate limits are hit
    - `link_expired` - Triggered when stream links expire

## YAML Structure

```yaml
proxy:
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

| Field                 | Type                               | Required | Description                              |
|-----------------------|------------------------------------|----------|------------------------------------------|
| `command`             | `gotemplate` or `[]gotemplate`     | No       | Default command for all error types      |
| `template_variables`  | [`[]NameValue`](#namevalue-object) | No       | Variables available in command templates |
| `env_variables`       | [`[]NameValue`](#namevalue-object) | No       | Environment variables for the command    |
| `upstream_error`      | `command`                          | No       | Command for upstream source failures     |
| `rate_limit_exceeded` | `command`                          | No       | Command for rate limit errors            |
| `link_expired`        | `command`                          | No       | Command for expired link errors          |

### Command Object

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

| Variable | Type     | Description          |
|----------|----------|----------------------|
| `url`    | `string` | Stream URL           |
| `reason` | `string` | Error reason message |

## Examples

### Default Error Handler

```yaml
proxy:
  error:
    command:
      - "ffmpeg"
      - "-v"
      - "error"
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
```

### Upstream Error with Test Pattern

```yaml
proxy:
  error:
    upstream_error:
      command:
        - "ffmpeg"
        - "-v"
        - "fatal"
        - "-f"
        - "lavfi"
        - "-i"
        - "testsrc2=size=1920x1080:rate=30"
        - "-c:v"
        - "libx264"
        - "-preset"
        - "fast"
        - "-t"
        - "300"
        - "-f"
        - "mpegts"
        - "pipe:1"
```

### Rate Limit Exceeded

```yaml
proxy:
  error:
    rate_limit_exceeded:
      template_variables:
        - name: message
          value: "Rate limit exceeded"
      command:
        - "ffmpeg"
        - "-v"
        - "error"
        - "-f"
        - "lavfi"
        - "-i"
        - "color=c=red:size=1280x720:duration=10"
        - "-f"
        - "lavfi"
        - "-i"
        - "sine=frequency=440:duration=1"
        - "-f"
        - "mpegts"
        - "pipe:1"
```

### Link Expired

```yaml
proxy:
  error:
    link_expired:
      command:
        - "ffmpeg"
        - "-v"
        - "error"
        - "-f"
        - "lavfi"
        - "-i"
        - "color=c=black:size=1280x720:duration=5"
        - "-f"
        - "lavfi"
        - "-i"
        - "sine=frequency=880:duration=0.5"
        - "-f"
        - "mpegts"
        - "pipe:1"
```

### Per-Error-Type Configuration

```yaml
proxy:
  error:
    upstream_error:
      command:
        - "ffmpeg"
        - "-v"
        - "error"
        - "-f"
        - "lavfi"
        - "-i"
        - "testsrc2=size=1280x720:rate=25"
        - "-f"
        - "mpegts"
        - "pipe:1"
    rate_limit_exceeded:
      command:
        - "ffmpeg"
        - "-v"
        - "error"
        - "-f"
        - "lavfi"
        - "-i"
        - "testsrc2=size=1280x720:rate=25:color=red"
        - "-f"
        - "lavfi"
        - "-i"
        - "sine=frequency=600:duration=2"
        - "-f"
        - "mpegts"
        - "pipe:1"
    link_expired:
      command:
        - "ffmpeg"
        - "-v"
        - "error"
        - "-f"
        - "lavfi"
        - "-i"
        - "testsrc2=size=1280x720:rate=25:color=gray"
        - "-f"
        - "lavfi"
        - "-i"
        - "sine=frequency=300:duration=2"
        - "-f"
        - "mpegts"
        - "pipe:1"
```
