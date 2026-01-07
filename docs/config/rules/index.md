# Rules

Rules allow you to modify, filter, and transform channels or channel lists using a flexible range of operations. Rules
are defined globally and applied to all clients, with optional filtering by client or playlist names.

## Rule Types

Rules are organized into two categories:

- **Channel Rules** - Operate on individual channels (set_field, remove_field, remove_channel, mark_hidden)
- **Playlist Rules** - Operate on the entire playlist/channel list (remove_duplicates, merge_channels, sort)

!!! note "Rule Processing"

* Rules can be filtered to specific channels, clients, or playlists using `condition` blocks.
* Channel rules are processed first, followed by playlist rules

## YAML Structure

```yaml
channel_rules: []
playlist_rules: []
```