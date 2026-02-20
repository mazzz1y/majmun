# EPGs

EPGs (Electronic Program Guides) define collections of TV program schedules from XML sources. Each EPG can contain
multiple sources.

## YAML Structure

```yaml
epgs:
  - name: ""
    sources: []
    proxy: {}
```

## Fields

| Field     | Type                  | Required | Description                                                 |
| --------- | --------------------- | -------- | ----------------------------------------------------------- |
| `name`    | `string`              | Yes      | Unique name identifier for this EPG                         |
| `sources` | `[]string`            | Yes      | List of EPG sources (URLs or file paths, XML or .gz).       |
| `proxy`   | [`Proxy`](./proxy.md) | No       | EPG-specific proxy configuration, only enabled takes effect |

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
