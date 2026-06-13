<div style="max-width: 850px; margin: 0 auto;" markdown>

# Install

## Releases

- Binary releases: [GitHub Releases](https://github.com/mazzz1y/majmun/releases)
- Docker image: [GitHub Packages](https://github.com/mazzz1y/majmun/pkgs/container/majmun)

## Quickstart

A minimal `config.yaml` — one playlist, one EPG, one client:

```yaml
server:
  listen_addr: ":8080"
  public_url: "http://localhost:8080"

url_generator:
  secret: "change-me"

playlists:
  - name: tv
    sources: "https://provider.com/playlist.m3u8"

epgs:
  - name: guide
    sources: "https://provider.com/guide.xml"

clients:
  - name: living-room
    secret: "living-room-secret"
```

Start Majmun with `majmun -config ./config.yaml`, then point your TV or player at:

- `http://localhost:8080/living-room-secret/playlist.m3u8` — the playlist
- `http://localhost:8080/living-room-secret/epg.xml` — the guide (`.gz` also available)

Each client gets its own URLs based on its `secret`. From here, see [Configuration](config.md) for all options and
[Examples](examples/index.md) for ready-to-use setups.

## Docker Compose Example

```yaml
services:
  majmun:
    image: ghcr.io/mazzz1y/majmun:latest
    restart: always
    command:
      - -config
      - /config
    volumes:
      - ./config:/config:ro
      - majmun-cache:/cache
      - majmun-state:/state
    tmpfs:
      - /tmp
    ports:
      - "8080:8080"

volumes:
  majmun-cache:
  majmun-state:
```

What each mount is for:

- **`/config`** — your configuration file(s).
- **`/cache`** — disk cache for playlists, guides, and logos. Point
  [`http_client.cache.path`](config/proxy/http_client.md) at it (e.g. `path: /cache`).
- **`/state`** — schedules for [generated channels](config/channels.md); set `playout.state_dir: /state` in your config.
  Without it, generated channels lose their live position on container recreation. Omit it if you don't use channels.
- **`/tmp` as tmpfs** — when proxying is enabled, Majmun writes temporary video segments to `/tmp`. Keeping it in
  RAM avoids disk wear.

!!! note "TLS"

    Majmun serves plain HTTP. If you expose it to the internet, put it behind a reverse proxy that terminates TLS
    (Nginx, Caddy, Traefik) and set [`public_url`](config/server.md) to the `https://` address.

!!! note "No GPU drivers in the image"

    The Docker image ships with FFmpeg but no GPU drivers or hardware-acceleration libraries (VAAPI, NVENC, QSV).
    To use [hardware acceleration](examples/playout.md#hardware-acceleration), extend the image with the
    drivers for your hardware and pass the devices through to the container.
