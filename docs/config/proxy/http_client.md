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

| Field     | Type                               | Required | Description                                 |
|-----------|------------------------------------|----------|---------------------------------------------|
| `cache`   | `object`                           | No       | Cache configuration for proxy requests      |
| `headers` | [`[]NameValue`](#namevalue-object) | No       | Extra request headers for outgoing requests |

### `http_client.cache`

| Field         | Type     | Required | Description                               |
|---------------|----------|----------|-------------------------------------------|
| `enabled`     | `bool`   | No       | Enable/disable disk cache (default: true) |
| `ttl`         | `string` | No       | Cache TTL (e.g., "5m", "1h")              |
| `retention`   | `string` | No       | Cache retention duration                  |
| `compression` | `bool`   | No       | Enable gzip compression for cached files  |

### Name/Value Object

| Field   | Type     | Required | Description                          |
|---------|----------|----------|--------------------------------------|
| `name`  | `string` | Yes      | Name identifier for the object       |
| `value` | `string` | Yes      | Value associated with the given name |

## Examples

### Basic HTTP Client Configuration

```yaml
proxy:
  http_client:
    cache:
      enabled: true
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
      ttl: 15m
      retention: 72h
      compression: true
    headers:
      - name: User-Agent
        value: "Mozilla/5.0 (compatible; Majmun Proxy)"
```
