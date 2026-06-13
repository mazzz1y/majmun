# Mark as Hidden

The `mark_hidden` rule excludes matching channels from [metrics](../../../metrics.md) and from channel names in logs.
The channels still play normally — use this to keep certain channels or clients out of your dashboards and log
output, e.g. for privacy or to cut monitoring noise.

## YAML Structure

```yaml
mark_hidden:
  condition: {}
```

## Fields

| Field       | Type                           | Required | Description                   |
| ----------- | ------------------------------ | -------- | ----------------------------- |
| `condition` | [`Condition`](../../shared/condition.md) | Yes      | Which channels will be hidden |

## Example

```yaml
channel_rules:
  - mark_hidden:
      condition:
        clients: ["top-client"]
```
