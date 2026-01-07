<div style="max-width: 850px; margin: 0 auto;" markdown>

# Configuration Examples

!!! note "Exposed endpoints"
    - `{public_url}/{client_secret}/playlist.m3u8`
    - `{public_url}/{client_secret}/epg.xml`
    - `{public_url}/{client_secret}/epg.xml.gz`

## Simple Configuration

A minimal setup with a basic server, one playlist, and one EPG.

```yaml
server:
  listen_addr: ":8080"
  public_url: "http://localhost:8080"

proxy:
  enabled: true # Proxy everything throw the gateway
  http_client:
    cache:
      enabled: true
      ttl: "5m"

playlists:
  - name: basic-tv
    sources: "https://provider.com/basic.m3u8"

epgs:
  - name: tv-guide
    sources: "https://provider.com/guide.xml"

clients:
  - name: "tv"
    secret: "tv-secret"

```

## Advanced Configuration

A full-featured setup including proxying, rules, and multiple sources.

```yaml
logs:
  level: debug
  format: json

server:
  listen_addr: ":8080"
  metrics_addr: ":9090"
  public_url: "https://iptv.example.com"

url_generator:
  secret: "super-secret"
  stream_ttl: "24h"
  file_ttl: "0s"

proxy:
  enabled: true
  concurrency: 10 # Set global concurrency
  http_client:
    cache:
      enabled: true
      path: /tmp/iptv/cache
      ttl: 15m
      retention: 72h
      compression: true
    headers:
      - name: User-Agent
        value: "My UA"
      - name: Authorization
        value: "Bearer ..."
  error:
    upstream_error:
      template_variables:
        - name: message
          value: |
            Canal temporalmente no disponible

            No hay respuesta del servidor ascendente

            Por favor, inténtelo más tarde
    rate_limit_exceeded:
      template_variables:
        - name: message
          value: |
            Se ha excedido el número de transmisiones simultáneas

            Por favor, inténtelo más tarde
    link_expired:
      template_variables:
        - name: message
          value: |
            El enlace del canal ha expirado

            Por favor, actualice la lista de reproducción en su televisor

playlists:
  - name: movies
    sources:
      - "https://provider.com/movies1.m3u8"
      - "https://provider.com/movies2.m3u8"
  - name: tv
    sources:
      - "https://provider.com/tv1.m3u8"
      - "https://provider.com/tv2.m3u8"
  - name: sports
    sources:
      - "https://provider.com/sports1.m3u8"
      - "https://provider.com/sports2.m3u8"

epgs:
  - name: movies
    sources:
      - "https://movies.com/guide.xml"
      - "https://movies2.com/guide.xml.gz"
  - name: tv
    sources:
      - "https://tv.com/guide.xml"
      - "https://tv2.com/guide.xml.gz"
  - name: sports
    sources:
      - "https://sports.com/guide.xml"
      - "https://sports2.com/guide.xml.gz"

channel_rules:
  # Set sports group for sports channels
  - set_field:
      selector: attr/group-title
      template: "Sports"
      condition:
        or:
          - playlists: ["sports"]
          - patterns: [".*ESPN.*", ".*Fox Sports.*", ".*Sky Sports.*"]

playlist_rules:
  # Remove duplicate channels, prefer highest quality for HD clients
  - remove_duplicates:
      patterns: ["4K", "FHD", "HD", "SD"]
      condition:
        clients: ["living-room", "bedroom"]

  # Remove duplicate channels, prefer SD quality for mobile/kitchen
  - remove_duplicates:
      patterns: ["SD", "HD", "FHD", "4K"]
      condition:
        clients: ["mobile", "kitchen"]

clients:
  - name: "living-room"
    secret: "lr-secret"
    playlists: ["tv", "movies"]

  - name: "bedroom"
    secret: "br-secret"
    playlists: ["sports", "tv"]

  - name: "mobile"
    secret: "mb-secret"
    playlists: ["tv", "movies"]

  - name: "kitchen"
    secret: "kt-secret"
    playlists: ["tv", "movies"]
```
