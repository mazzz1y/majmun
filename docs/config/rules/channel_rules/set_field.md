# Set Field

The `set_field` rule allows you to modify channel properties including the channel name, M3U tags, and attributes.

## YAML Structure

```yaml
set_field:
  selector: {}
  template: ""
  condition: {}
```

## Fields

| Field       | Type                           | Required | Description                               |
| ----------- | ------------------------------ | -------- | ----------------------------------------- |
| `selector`  | [`Selector`](../selector.md)   | Yes      | What property to set (attribute/tag/name) |
| `template`  | `gotemplate`                   | Yes      | The template definition for the new value |
| `condition` | [`Condition`](../condition.md) | No       | Optional, restricts rule activation       |

## Template Variables

!!! note "Error handling"
If the template refers to `nil` or if any other runtime template execution error occurs,
playlist generation will fail.

| Variable                  | Type                | Description                                |
| ------------------------- | ------------------- | ------------------------------------------ |
| `{{.Channel.Name}}`       | string              | The original channel name.                 |
| `{{.Channel.Attrs}}`      | `map[string]string` | A map containing the channel's attributes. |
| `{{.Channel.Tags}}`       | `map[string]string` | A map containing the channel's tags.       |
| `{{.Playlist.Name}}`      | string              | The channel's playlist name.               |
| `{{.Playlist.IsProxied}}` | bool                | Indicates whether the playlist is proxied. |

## Examples

Set channel group to `Free` for all channels in the `custom` playlist:

```yaml
channel_rules:
  - set_field:
      selector: tag/EXTGRP
      template: "Free"
      condition:
        playlists: custom
```
