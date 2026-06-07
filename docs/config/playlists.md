# Playlists

Playlists define collections of IPTV channels from M3U/M3U8 sources. Each playlist can contain multiple sources with
proxy configuration.

## YAML Structure

```yaml
playlists:
  - name: "playlist-name"
    sources: []
    proxy: {}
    skip_on_error: false
```

## Fields

| Field           | Type                  | Required | Description                                                                                                                                                                                                              |
| --------------- | --------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `name`          | `string`              | Yes      | Unique name identifier for this playlist                                                                                                                                                                                 |
| `sources`       | `[]string`            | Yes      | List of playlist sources (URLs or file paths, M3U/M3U8 format).                                                                                                                                                          |
| `proxy`         | [`Proxy`](./proxy.md) | No       | Playlist-specific proxy configuration                                                                                                                                                                                    |
| `skip_on_error` | `bool`                | No       | When `true`, a source that errors (load failure, non-2xx, or mid-stream decode error) is logged and skipped instead of aborting the response. Channels read before the failure are kept; the remainder is dropped. Default `false`. |

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

### Skip Failing Sources

When `skip_on_error: true` is set, an upstream source that returns an error (network failure,
non-2xx status, decode error) is logged and skipped instead of aborting the whole response.
Useful for non-priority or free providers that are occasionally unreliable. Any channels read
before a mid-stream failure are kept; the rest of that source is dropped. If all sources fail,
the request still errors out with `no channels found in subscriptions`.

```yaml
playlists:
  - name: combined
    sources:
      - "https://reliable-provider.com/channels.m3u8"
      - "https://flaky-provider.com/channels.m3u8"
    skip_on_error: true
```
