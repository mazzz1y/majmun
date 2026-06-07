# EPGs

EPGs (Electronic Program Guides) define collections of TV program schedules from XML sources. Each EPG can contain
multiple sources.

## YAML Structure

```yaml
epgs:
  - name: ""
    sources: []
    proxy: {}
    skip_on_error: false
```

## Fields

| Field           | Type                  | Required | Description                                                                                                                                                                                                                                  |
| --------------- | --------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`          | `string`              | Yes      | Unique name identifier for this EPG                                                                                                                                                                                                          |
| `sources`       | `[]string`            | Yes      | List of EPG sources (URLs or file paths, XML or .gz).                                                                                                                                                                                        |
| `proxy`         | [`Proxy`](./proxy.md) | No       | EPG-specific proxy configuration, only enabled takes effect                                                                                                                                                                                  |
| `skip_on_error` | `bool`                | No       | When `true`, a source that errors (load failure, non-2xx, or mid-stream decode error) is logged and skipped instead of aborting the response. Channels and programmes read before the failure are kept; the remainder is dropped. Default `false`. |

## Examples

### Basic EPG

```yaml
epgs:
  - name: tv-guide
    sources:
      - "https://provider.com/guide.xml"
```

### Multi-Source EPG

```yaml
epgs:
  - name: combined-guide
    sources:
      - "https://provider-1.com/epg.xml.gz"
      - "https://provider-2.com/schedule.xml"
      - "/local/custom-guide.xml"
```

### EPG with Proxy

```yaml
epgs:
  - name: international-guide
    sources:
      - "https://international-provider.com/epg.xml"
    proxy:
      enabled: true
```

### Skip Failing Sources

When `skip_on_error: true` is set, an upstream EPG source that errors out (network failure,
non-2xx status, decode error) is logged and skipped instead of aborting the whole response.
Useful for non-priority or free EPG providers. Any channels/programmes read before a mid-stream
failure are kept; the rest of that source is dropped. If all sources fail, the request still
errors out with `no data in subscriptions`.

```yaml
epgs:
  - name: combined-guide
    sources:
      - "https://reliable-provider.com/epg.xml"
      - "https://flaky-provider.com/epg.xml"
    skip_on_error: true
```
