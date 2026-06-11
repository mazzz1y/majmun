# Playlists

Playlists define collections of IPTV channels from M3U/M3U8 sources. A playlist can also generate its own 24/7 linear
channels from local files via the `channels` block. Each playlist can contain multiple sources with proxy configuration.

A playlist must define at least one of `sources` or `channels`, and may define both to mix remote channels with locally
generated ones.

## YAML Structure

```yaml
playlists:
  - name: "playlist-name"
    sources: []
    channels: []
    proxy: {}
    skip_on_error: false
```

## Fields

| Field           | Type                  | Required | Description                                                                                                                                                                                                              |
| --------------- | --------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `name`          | `string`              | Yes      | Unique name identifier for this playlist                                                                                                                                                                                 |
| `sources`       | `[]string`            | No\*     | List of playlist sources (URLs or file paths, M3U/M3U8 format).                                                                                                                                                          |
| `channels`      | [`[]Channel`](./channels.md) | No\* | Generated 24/7 linear channels from local files. See [Channels](./channels.md).                                                                                                                                   |
| `proxy`         | [`Proxy`](./proxy.md) | No       | Playlist-specific proxy configuration. Cascades to the playlist's channels (including [`playout`](./proxy/playout.md)).                                                                                                  |
| `skip_on_error` | `bool`                | No       | When `true`, a failing source is skipped instead of aborting the response. Default `false`. See [Skip Failing Sources](#skip-failing-sources). |

\* At least one of `sources` or `channels` is required.

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

### Playlist with Generated Channels

A playlist can carry locally generated channels alongside (or instead of) remote sources. See
[Channels](./channels.md) for the channel fields.

```yaml
state_dir: ./state

playlists:
  - name: local
    channels:
      - name: "Cartoons 24/7"
        fields:
          - selector: attr/group-title
            template: "Kids"
        sources:
          - /media/cartoons
  - name: mixed
    sources:
      - "https://provider.com/list.m3u8"
    channels:
      - name: "Movie Marathon"
        sources: [/media/movies]
        random_order: true
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
