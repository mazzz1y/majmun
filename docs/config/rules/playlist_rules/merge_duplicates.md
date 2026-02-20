# Merge Duplicates

The `merge_duplicates` rule combines channels considered duplicates (e.g., "CNN HD", "CNN 4K") into one logical channel
with fallback sources.

It is similar to the `remove_duplicates` rule, but instead of removing duplicates, it treats duplicates as fallback
sources when a channel is unavailable for some reason, such as upstream errors or concurrency limits for the playlist.

## YAML Structure

```yaml
merge_duplicates:
  patterns: []
  selector: ""
  final_value: {}
  condition: {}
```

## Fields

| Field       | Type                              | Required | Description                                 |
|-------------|-----------------------------------|----------|---------------------------------------------|
| `patterns`    | `[]regex`                         | Yes      | Priority order (first has highest priority) |
| `selector`    | [`Selector`](../selector.md)      | No       | Use attribute or tag to identify groups     |
| `final_value` | [`FinalValue`](../final_value.md) | No       | Use for modify result channels              |
| `condition`   | [`Condition`](../condition.md)    | No       | Only apply if condition matches             |

## Examples

Prefer the best quality:

```yaml
# Input: CNN HD, CNN 4K
# Output: CNN HQ (with fallback to CNN HD)

playlist_rules:
  - merge_duplicates:
      patterns: ["4K", "UHD", "FHD", "HD", "SD"]
      final_value:
        template: "{{.BaseName}} HQ"
```

Restrict merging to specific clients:

```yaml
# Input: CNN HD, CNN 4K, CNN SD
# Output: CNN SD (with fallbacks to CNN HD, CNN 4K)

playlist_rules:
  - merge_duplicates:
      patterns: ["SD", "HD", "FHD", "4K"]
      condition:
        clients: ["kitchen", "office-lite"]
```

