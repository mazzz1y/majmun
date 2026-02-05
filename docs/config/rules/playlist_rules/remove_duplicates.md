# Remove Duplicates

The `remove_duplicates` rule keeps only one channel among groups considered duplicates (e.g., preferring "CNN HD" over "
CNN").

## YAML Structure

```yaml
remove_duplicates:
  patterns: []
  selector: ""
  final_value: {}
  condition: {}
```

## Fields

| Field       | Type                              | Required | Description                                 |
|-------------|-----------------------------------|----------|---------------------------------------------|
| patterns    | `[]regex`                         | Yes      | Priority order (first has highest priority) |
| selector    | [`Selector`](../selector.md)      | No       | Use attribute or tag to identify groups     |
| final_value | [`FinalValue`](../final_value.md) | No       | Use for modify result channels              |
| condition   | [`Condition`](../condition.md)    | No       | Only apply if condition matches             |

## Examples

Prefer the best quality:

```yaml
# Input: CNN HD, CNN 4K
# Output: CNN HQ
playlist_rules:
  - remove_duplicates:
      patterns: ["4K", "UHD", "FHD", "HD", "SD", ""]
      final_value:
        template: "{{.BaseName}} HQ"
```

Restrict deduplication to specific clients:

```yaml
# Input: CNN HD, CNN 4K, CNN SD
# Output: CNN SD
playlist_rules:
  - remove_duplicates:
      patterns: ["SD", "HD", "FHD", "4K"]
      condition:
        clients: ["kitchen", "office-lite"]
```
