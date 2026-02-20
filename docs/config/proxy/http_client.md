# HTTP Client

The `http_client` block configures outgoing HTTP requests made by Majmun for proxy operations.
It includes optional disk caching for playlists, EPG data, and proxied assets (images, etc.).

!!! note "Proxy HTTP Client"

    The proxy-level HTTP client configuration overrides the global HTTP client settings for this specific proxy.

## YAML Structure

```yaml
proxy:
  http_client:
    cache:
      enabled: true
      ttl: 15m
      retention: 72h
      compression: true
    headers: []
```

## Fields

### `http_client`

| Field     | Type                                           | Required | Description                                 |
| --------- | ---------------------------------------------- | -------- | ------------------------------------------- |
| `cache`   | `object`                                       | No       | Cache configuration for proxy requests      |
| `headers` | [`[]NameValue`](../shared.md#namevalue-object) | No       | Extra request headers for outgoing requests |

### `http_client.cache`

| Field         | Type                                | Required               | Description                               |
| ------------- | ----------------------------------- | ---------------------- | ----------------------------------------- |
| `enabled`     | `bool`                              | No                     | Enable/disable disk cache (default: true) |
| `path`        | `string`                            | Yes (if cache enabled) | Path to cache directory (global only)     |
| `ttl`         | [`duration`](../shared.md#duration) | Yes (if cache enabled) | Cache TTL (e.g., "5m", "1h")              |
| `retention`   | [`duration`](../shared.md#duration) | Yes (if cache enabled) | Cache retention duration                  |
| `compression` | `bool`                              | No                     | Enable gzip compression for cached files  |

## Examples

### Basic HTTP Client Configuration

```yaml
proxy:
  http_client:
    cache:
      enabled: true
      path: /tmp/cache
      ttl: 5m
      retention: 24h
```

### With Custom Headers

```yaml
proxy:
  http_client:
    headers:
      - name: User-Agent
        value: "Majmun/1.0"
      - name: X-Proxy-ID
        value: "proxy-01"
```

### With Compression

```yaml
proxy:
  http_client:
    cache:
      enabled: true
      path: /tmp/cache
      ttl: 10m
      retention: 48h
      compression: true
```

### Full Configuration

```yaml
proxy:
  http_client:
    cache:
      enabled: true
      path: /tmp/cache
      ttl: 15m
      retention: 72h
      compression: true
    headers:
      - name: User-Agent
        value: "Mozilla/5.0 (compatible; Majmun Proxy)"
```
