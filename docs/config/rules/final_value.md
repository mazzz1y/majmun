# Final Value

Final value allows customizing the result channel after merging/removing duplicates. Used in deduplication rules.

## YAML Structure

```yaml
final_value:
  selector: {}
  template: ""
```

## Fields

| Field      | Type                        | Required | Description                         |
| ---------- | --------------------------- | -------- | ----------------------------------- |
| `selector` | [`Selector`](./selector.md) | No       | Property to modify on the result    |
| `template` | `gotemplate`                | No       | Go template for the resulting value |

## Template Variables

!!! note "Error handling"

    If the template refers to `nil` or if any other runtime template execution error occurs,
    playlist generation will fail.

In deduplication, the "winning" channel is the one whose value matched the highest-priority pattern (first in the
`patterns` list). The template runs against that channel:

| Variable                  | Type                | Description                                                                              |
| ------------------------- | ------------------- | ----------------------------------------------------------------------------------------- |
| `{{.Channel.Name}}`       | string              | The winning channel's original name.                                                     |
| `{{.Channel.Attrs}}`      | `map[string]string` | A map containing the channel's attributes.                                                |
| `{{.Channel.Tags}}`       | `map[string]string` | A map containing the channel's tags.                                                      |
| `{{.Channel.BaseName}}`   | string              | The name with all `patterns` stripped — the part duplicates share (e.g. `CNN` for `CNN HD` / `CNN 4K`). |
| `{{.Playlist.Name}}`      | string              | The winning channel's playlist name.                                                      |
| `{{.Playlist.IsProxied}}` | bool                | Whether the winning channel's playlist is proxied.                                        |
