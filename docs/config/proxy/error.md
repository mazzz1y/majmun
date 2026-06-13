# Error

When a proxied stream can't be served, Majmun doesn't just drop the connection — it runs an error command that
generates a short video telling the viewer what happened. The default renders a color card with an explanatory
message; you can replace it with any command per error type.

There are three error types:

- `upstream_error` — the upstream source failed (unreachable, error response, or the stream command produced no
  output).
- `rate_limit_exceeded` — a [`concurrency`](../proxy.md#configuration-levels) limit (global, playlist, or client)
  was hit.
- `link_expired` — the stream link outlived [`url_generator.stream_ttl`](../url_generator.md); the viewer needs to
  refresh the playlist.

Error commands follow the same output contract as the [stream](./stream.md) command: write video data to `stdout`
(`pipe:1` for FFmpeg); `stderr` goes to the debug logs.

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

| Field                 | Type                                           | Required | Description                              |
| --------------------- | ---------------------------------------------- | -------- | ---------------------------------------- |
| `command`             | [`Command`](../shared/command.md)                     | No       | Default command for all error types      |
| `template_variables`  | [`[]NameValue`](../shared/name-value.md) | No       | User variables for the command           |
| `env_variables`       | [`[]NameValue`](../shared/name-value.md) | No       | Environment variables for the command    |
| `upstream_error`      | [`CommandObject`](#command-object)             | No       | Command for upstream source failures     |
| `rate_limit_exceeded` | [`CommandObject`](#command-object)             | No       | Command for rate limit errors            |
| `link_expired`        | [`CommandObject`](#command-object)             | No       | Command for expired link errors          |

### Command Object

| Field                | Type                                           | Required | Description                              |
| -------------------- | ---------------------------------------------- | -------- | ---------------------------------------- |
| `command`            | [`Command`](../shared/command.md)                     | No       | Command array to execute                 |
| `template_variables` | [`[]NameValue`](../shared/name-value.md) | No       | User variables for the command           |
| `env_variables`      | [`[]NameValue`](../shared/name-value.md) | No       | Environment variables for the command    |

### Default Template Variables

The default command renders a static color card with the error text. These variables are available to it and can be
overridden per error type:

| Variable           | Default                                                            | Description                          |
| ------------------ | ------------------------------------------------------------------ | ------------------------------------ |
| `ffmpeg_log_level` | `fatal`                                                            | ffmpeg `-v` log level                |
| `message`          | Per error type, e.g. `Link has expired\n\nPlease refresh your playlist` | Text drawn on the error card    |

## Examples

### Custom Error Message

```yaml
proxy:
  error:
    link_expired:
      template_variables:
        - { name: message, value: "Your link is no longer valid" }
```

### Upstream Error with Test Pattern

```yaml
proxy:
  error:
    upstream_error:
      command:
        [ffmpeg, -v, fatal, -f, lavfi, -i, "testsrc2=size=1920x1080:rate=30",
         -c:v, libx264, -preset, fast, -t, "300", -f, mpegts, "pipe:1"]
```

The same approach works for the other error types — the `command`/`template_variables`/`env_variables` blocks are
identical under `rate_limit_exceeded` and `link_expired`, and a top-level `command` replaces the default for all
three at once.
