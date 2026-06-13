<div style="max-width: 850px; margin: 0 auto;" markdown>

# Full Setup

Everything together: proxied playlists with caching and error screens, generated local channels, global and per-client
rules, and multiple clients with different playlist sets.

```yaml
logs:
  level: debug
  format: json

server:
  listen_addr: ":8080"
  metrics_addr: ":9090"
  public_url: "https://iptv.example.com"

playout:
  state_dir: /var/lib/majmun/state # where generated-channel schedules are persisted

url_generator:
  secret: "super-secret"
  stream_ttl: "24h"
  file_ttl: "0s"

proxy:
  enabled: true
  concurrency: 10 # global cap on simultaneous upstream streams
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
  - name: local
    channels:
      - name: "Cartoons 24/7"
        # 24/7 channel generated from local media files
        fields:
          - selector: attr/group-title
            template: "Kids"
        sources:
          - /media/cartoons
        playout:
          logo: /config/logos/cartoons.png
      - name: "Comedy Shows"
        # Episodes play in random order, reshuffled when the file set changes
        sources:
          - /media/shows/comedy
        playout:
          random_order: true

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
    playlists: ["tv", "movies", "local"]

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

</div>
