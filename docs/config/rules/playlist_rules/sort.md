# Sort Playlist Rule

The `sort` rule controls the ordering of channels in a playlist, optionally grouping by any channel field.

## YAML Structure

```yaml
sort:
  selector: ""
  order: []
  group_by:
    selector: ""
    group_order: []
  condition: {}
```

## Fields

| Field       | Type                           | Required | Description                                                      |
|-------------|--------------------------------|----------|------------------------------------------------------------------|
| `selector`  | [`Selector`](../selector.md)   | No       | Property to use for sorting (attribute/tag/etc), default is name |
| `order`     | `[]regex`                      | No       | Custom order of channels, regex patterns                         |
| `group_by`  | [`GroupByRule`](#groupbyrule)  | No       | Group before sorting                                             |
| `condition` | [`Condition`](../condition.md) | No       | Only `clients` field is allowed in sort condition                |

### GroupByRule

| Field         | Type                         | Required | Description                            |
|---------------|------------------------------|----------|----------------------------------------|
| `selector`    | [`Selector`](../selector.md) | Yes      | How to group (attribute/tag)           |
| `group_order` | `[]regex`                    | No       | Custom order of groups, regex patterns |

## How It Works

1. If `group_by` is set, channels are grouped by `group_by.selector`.
2. Channel or group order is determined by the corresponding `order` arrays (if present).
3. Within each group (or globally), regex `order` is applied in order. Unmatched go at the end.
4. Channels within the same priority are alphabetically sorted by name/selector value.

## Examples

Simple alphabetical sort:

```yaml
playlist_rules:
  - sort: {}
```

Sort by attribute, move News and Children groups to the top:

```yaml
playlist_rules:
  - sort:
      selector: attr/tvg-name
      group_by:
        selector: tag/EXTGRP
        group_order: ["News", "Children", ""]
```
