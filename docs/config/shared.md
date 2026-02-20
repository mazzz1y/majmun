# Shared Types

Common types used across multiple configuration sections.

## Duration

Duration values support the following units:

- `s` - seconds
- `m` - minutes
- `h` - hours
- `d` - days (24 hours)
- `w` - weeks (7 days)
- `M` - months (30 days)
- `y` - years (365 days)

Examples: `30s`, `5m`, `2h`, `1d`, `2w`

## Name/Value Object

A key-value pair used for template variables, environment variables, and HTTP headers.

| Field   | Type     | Required | Description                          |
| ------- | -------- | -------- | ------------------------------------ |
| `name`  | `string` | Yes      | Name identifier for the object       |
| `value` | `string` | Yes      | Value associated with the given name |

## Command

Commands can be specified as a single string or an array of strings. Each element supports [Go template](https://pkg.go.dev/text/template) syntax with [Sprig functions](https://masterminds.github.io/sprig/).

**Array form** — each element is a separate argument:

```yaml
command:
  - "ffmpeg"
  - "-v"
  - "{{ .log_level }}"
  - "-i"
  - "{{ .url }}"
```

**String form** — passed to `sh -c`:

```yaml
command: "ffmpeg -v {{ .log_level }} -i {{ .url }} -c copy -f mpegts pipe:1"
```
