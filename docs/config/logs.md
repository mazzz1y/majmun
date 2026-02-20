# Logs

Log configuration controls the application's logging behavior.

## YAML Structure

```yaml
logs:
  level: ""
  format: ""
```

## Fields

| Field    | Type     | Required | Description                              |
| -------- | -------- | -------- | ---------------------------------------- |
| `level`  | `string` | No       | Logging level (debug, info, warn, error) |
| `format` | `string` | No       | Log output format (text, json)           |

## Log Levels

| Level   | Description                                             |
| ------- | ------------------------------------------------------- |
| `debug` | Most verbose level, includes all log messages           |
| `info`  | General information messages (default)                  |
| `warn`  | Warning messages for potentially problematic situations |
| `error` | Error messages for serious problems                     |

## Log Formats

| Format | Description                          |
| ------ | ------------------------------------ |
| `text` | Human-readable text format (default) |
| `json` | JSON structured logging format       |
