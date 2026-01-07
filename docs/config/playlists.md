# Playlists

Playlists define collections of IPTV channels from M3U/M3U8 sources. Each playlist can contain multiple sources with
proxy configuration.

## YAML Structure

```yaml
playlists:
  - name: "playlist-name"
    sources: []
    proxy: {}
```

## Fields

| Field     | Type                | Required | Description                                                     |
|-----------|---------------------|----------|-----------------------------------------------------------------|
| `name`    | `string`            | Yes      | Unique name identifier for this playlist                        |
| `sources` | `[]string`          | Yes      | List of playlist sources (URLs or file paths, M3U/M3U8 format). |
| `proxy`   | [Proxy](./proxy.md) | No       | Playlist-specific proxy configuration                           |

## Examples

### Basic Playlist

```yaml
playlists:
  - name: basic-tv
    sources:
      - "https://provider.com/basic.m3u8"
```

### Multi-Source Playlist

```yaml
playlists:
  - name: sports-premium
    sources:
      - "https://sports-provider.com/premium.m3u8"
      - "https://sports-provider.com/international.m3u8"
      - "/local/custom-playlist.m3u8"
```

### Playlist with Proxy Configuration

```yaml
playlists:
  - name: premium-channels
    sources:
      - "https://premium-provider.com/channels.m3u8"
    proxy:
      enabled: true
      concurrency: 5
```
