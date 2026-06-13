<div style="max-width: 850px; margin: 0 auto;" markdown>

# Proxy

Restream upstream channels through majmun so the provider sees one connection per channel regardless of viewer count.
This config adds response caching, custom upstream headers, and localized error screens.

```yaml
server:
  listen_addr: ":8080"
  public_url: "https://iptv.example.com"

url_generator:
  secret: "super-secret"
  stream_ttl: "24h"

proxy:
  enabled: true
  concurrency: 10 # max simultaneous upstream streams
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

            Por favor, actualice la lista de reproducción

playlists:
  - name: tv
    sources:
      - "https://provider.com/tv1.m3u8"
      - "https://provider.com/tv2.m3u8"

clients:
  - name: "living-room"
    secret: "lr-secret"
    playlists: ["tv"]
```

See also: [Proxy](../config/proxy.md), [Stream](../config/proxy/stream.md),
[Segmenter](../config/proxy/segmenter.md), [Error](../config/proxy/error.md),
[HTTP Client](../config/proxy/http_client.md).

</div>
