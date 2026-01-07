# HTTP Client

The `http_client` block configures outgoing HTTP requests made by Majmun.
It includes optional disk caching for playlist/EPG downloads and file proxying.

## Key Concepts

- Cache TTL controls freshness. When the TTL expires, Majmun will attempt to renew the cache. If the resource is
  unchanged
  (based on `Expires`, `Last-Modified`, and `ETag` headers), the TTL will be renewed.
- Retention controls cleanup (how long unaccessed files stay on disk).
- Compression can be enabled to reduce disk usage by gzipping cached files.

## YAML Structure

```yaml
proxy:
  http_client:
    cache:
      enabled: true
      path: /tmp/iptv/cache
      ttl: 15m
      retention: 72h
      compression: true
    headers: []
```

## Fields

### `http_client`

| Field     | Type                               | Required | Description                                 |
|-----------|------------------------------------|----------|---------------------------------------------|
| `cache`   | `object`                           | Yes      | Cache configuration                         |
| `headers` | [`[]NameValue`](#namevalue-object) | No       | Extra request headers for outgoing requests |

### `http_client.cache`

| Field         | Type     | Required | Description                                     |
|---------------|----------|----------|-------------------------------------------------|
| `enabled`     | `bool`   | Yes      | Enable/disable disk cache globally              |
| `path`        | `string` | Yes*     | Cache directory (required when `enabled: true`) |
| `ttl`         | `string` | Yes*     | Cache TTL (required when `enabled: true`)       |
| `retention`   | `string` | Yes*     | Cache retention (required when `enabled: true`) |
| `compression` | `bool`   | No       | Enable gzip compression for cached files        |

### Name/Value Object

| Field   | Type     | Required | Description                          |
|---------|----------|----------|--------------------------------------|
| `name`  | `string` | Yes      | Name identifier for the object       |
| `value` | `string` | Yes      | Value associated with the given name |
