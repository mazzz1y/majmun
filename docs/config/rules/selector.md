# Selector

The `Selector` type is used to specify which channel property to target for operations like setting or filtering. It is
defined in a single-line format to select a specific field.

!!! note
If selector is not specified, the default selector is `name`.

## Possible Values

| Format             | Description                                                     |
|--------------------|-----------------------------------------------------------------|
| `name`             | Targets the channel name                                        |
| `attr/<attribute>` | Targets a specific channel attribute (e.g., `attr/group-title`) |
| `tag/<tag>`        | Targets a specific M3U tag (e.g., `tag/EXTGRP`)                 |

## Example

Set EXTGRP tag to "News" for the "news" playlist:

```yaml
set_field:
  selector: tag/EXTGRP
  template: "News"
  condition:
    playlists: news
```
