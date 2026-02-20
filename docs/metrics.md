<div style="max-width: 850px; margin: 0 auto;" markdown>

# Metrics

Majmun provides Prometheus metrics for monitoring system performance, stream statistics, and client activity.

## YAML Configuration

```yaml
server:
  metrics_addr: ""
```

## Configuration Fields

| Field          | Type     | Required | Default | Description                         |
| :------------- | :------- | :------- | :------ | :---------------------------------- |
| `metrics_addr` | `string` | No       | `""`    | Address and port for metrics server |

## Available Metrics

### Stream Metrics

| Metric Name                    | Type    | Description                       | Labels                                                   |
| ------------------------------ | ------- | --------------------------------- | -------------------------------------------------------- |
| `iptv_playlist_streams_active` | Gauge   | Currently active playlist streams | `playlist_name`                                          |
| `iptv_client_streams_active`   | Gauge   | Currently active client streams   | `client_name`, `playlist_name`, `channel_name`           |
| `iptv_streams_reused_total`    | Counter | Total number of reused streams    | `playlist_name`, `channel_name`                          |
| `iptv_streams_failures_total`  | Counter | Total number of stream failures   | `client_name`, `playlist_name`, `channel_name`, `reason` |

### Request Metrics

| Metric Name                    | Type    | Description                                | Labels                                        |
| ------------------------------ | ------- | ------------------------------------------ | --------------------------------------------- |
| `iptv_listing_downloads_total` | Counter | Total listing downloads by client and type | `client_name`, `request_type`                 |
| `iptv_proxy_requests_total`    | Counter | Total proxy requests by client and status  | `client_name`, `request_type`, `cache_status` |

## Common Label Values

| Label           | Description                                     | Possible Values                                                    |
| --------------- | ----------------------------------------------- | ------------------------------------------------------------------ |
| `client_name`   | Unique identifier for each client configuration | any                                                                |
| `playlist_name` | Name of the playlist being accessed             | any                                                                |
| `channel_name`  | Name of individual channels                     | any                                                                |
| `request_type`  | Type of request                                 | `playlist`, `epg`, `file`                                          |
| `cache_status`  | Cache hit status                                | `hit`, `miss`, `renewed`                                           |
| `reason`        | Failure reason                                  | `global_limit`, `playlist_limit`, `client_limit`, `upstream_error` |
