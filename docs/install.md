<div style="max-width: 850px; margin: 0 auto;" markdown>

# Install

## Releases

- Binary releases: [GitHub Releases](https://github.com/mazzz1y/majmun/releases)
- Docker image: [GitHub Packages](https://github.com/mazzz1y/majmun/pkgs/container/majmun)

## Docker Compose Example

This is a minimal example. Majmun is oriented toward experienced users and expects that you know what you're doing.

If the application is publicly available, it requires a proper TLS setup. Refer to the Nginx, Caddy, or Traefik
documentation.

For configuration, see [Configuration](config.md) and [Examples](examples.md).

!!! tip "tmpfs for Segments"

    The HLS segmenter writes temporary segment files to `/tmp`. To avoid disk wear, mount `/tmp` as tmpfs.

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
    tmpfs:
      - /tmp
    ports:
      - "8080:8080"

volumes:
  majmun-cache:
```
