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

| Variable                  | Type                | Description                                               |
| ------------------------- | ------------------- | --------------------------------------------------------- |
| `{{.Channel.Name}}`       | string              | The original channel name.                                |
| `{{.Channel.Attrs}}`      | `map[string]string` | A map containing the channel's attributes.                |
| `{{.Channel.Tags}}`       | `map[string]string` | A map containing the channel's tags.                      |
| `{{.Channel.BaseName}}`   | string              | Duplicates basename                                       |
| `{{.Playlist.Name}}`      | string              | The best channel's playlist name.                         |
| `{{.Playlist.IsProxied}}` | bool                | Indicates whether the best channel's playlist is proxied. |
