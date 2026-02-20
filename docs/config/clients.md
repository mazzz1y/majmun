# Clients

The clients block represents a list of IPTV clients. Each client typically corresponds to a single device or user
accessing the IPTV service. Clients are identified by their unique name and authenticated using a secret key.

## Client Links

Each client can access the following endpoints:

- `{public_url}/{client_secret}/playlist.m3u8`
- `{public_url}/{client_secret}/epg.xml`
- `{public_url}/{client_secret}/epg.xml.gz`

!!! note
    If playlists/epgs are not explicitly configured for a client, it means that all sources are enabled.

## YAML Structure

```yaml
clients:
  - name: ""
    secret: ""
    proxy: {}
    playlists: []
    epgs: []
```

## Fields

| Field       | Type       | Required | Description                              |
|-------------|------------|----------|------------------------------------------|
| `name`      | `string`   | Yes      | Unique name identifier for this client   |
| `secret`    | `string`   | Yes      | Authentication secret key for the client |
| `playlists` | `[]string` | No       | List of playlist names for this client.   |
| `epgs`      | `[]string` | No       | List of EPG names for this client.        |
| `proxy`     | [`Proxy`](./proxy.md) | No       | Optional per-client proxy config         |

## Examples

### Basic Client Configuration

```yaml
clients:
  - name: living-room-tv
    secret: "living-room-secret-123"
    playlists: "sports-playlist"
```

### Client with Multiple Playlists and EPGs

```yaml
clients:
  - name: family-tablet
    secret: "family-tablet-secret-456"
    playlists: ["basic-playlist", "kids-playlist"]
    epgs: ["main-epg", "kids-epg"]
```

### Client with Proxy Configuration

```yaml
clients:
  - name: streaming-device
    secret: "streaming-device-secret-789"
    playlists: ["premium-channels"]
    proxy:
      enabled: true
      concurrency: 2
```
