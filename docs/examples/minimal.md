<div style="max-width: 850px; margin: 0 auto;" markdown>

# Minimal

The smallest working setup: one upstream playlist, one EPG, and one client. No proxying — links point directly at the
upstream.

```yaml
server:
  listen_addr: ":8080"
  public_url: "http://localhost:8080"

url_generator:
  secret: "your-secret-key"

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

The `tv` client's playlist is then served at `http://localhost:8080/tv-secret/playlist.m3u8`.

See also: [Server](../config/server.md), [Playlists](../config/playlists.md), [EPGs](../config/epgs.md),
[Clients](../config/clients.md), [URL Generator](../config/url_generator.md).

</div>
