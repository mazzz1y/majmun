# Configuration

Majmun can read configuration from a file or from a directory by combining multiple files based on top-level
elements.

By default, it reads configuration from `config.yaml` in the current directory.

```bash
majmun -config ./config.yaml # from file
majmun -config ./config      # from directory
```

!!! note "Hint"

    All arrays with a single value can be specified without brackets.

### Root Level Configuration

| Field           | Type                                       | Description                                                       |
|-----------------|--------------------------------------------|-------------------------------------------------------------------|
| `server`        | [Server](./config/server.md)               | Server configuration including listening addresses and public URL |
| `url_generator` | [URL Generator](./config/url_generator.md) | URL generation and encryption configuration                       |
| `logs`           | [Logs](config/logs.md)                      | Logging configuration                                             |
| `proxy`         | [Proxy](./config/proxy.md)                 | Stream proxy configuration for remuxing with ffmpeg               |
| `http_client`   | [HTTP Client](./config/http_client.md)     | HTTP client configuration (headers, caching)                      |
| `playlists`     | [Playlists](./config/playlists.md)         | Array of playlist definitions with sources                        |
| `epgs`          | [EPGs](./config/epgs.md)                   | Array of EPG definitions with sources                             |
| `channel_rules` | [Channel Rules](./config/rules/index.md)    | Global channel processing rules (applied to all channels)         |
| `playlist_rules`| [Playlist Rules](./config/rules/index.md)   | Global playlist processing rules (applied after channel rules)     |
| `clients`       | [Clients](./config/clients.md)             | Array of IPTV client definitions with individual settings         |
