# Mark as Hidden

The `mark_hidden` rule marks channels as hidden from metrics and logs.

## YAML Structure

```yaml
mark_hidden:
  condition: {}
```

## Fields

| Field     | Type                           | Required | Description                   |
|-----------|--------------------------------|----------|-------------------------------|
| condition | [`Condition`](../condition.md) | Yes      | Which channels will be hidden |

## Example

```yaml
channel_rules:
  - mark_hidden:
      condition:
        clients: ["top-client"]
```
