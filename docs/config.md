# Configuration

Majmun reads configuration from a single YAML file or from a directory. With a directory, all `.yaml`/`.yml` files
are loaded in alphabetical order into one configuration — useful for splitting playlists, clients, and rules into
separate files. A top-level key defined in a later file overrides the same key from an earlier one.

By default, it reads configuration from `config.yaml` in the current directory.

```bash
majmun -config ./config.yaml # from file
majmun -config ./config      # from directory
```

!!! info "Single-Value Arrays"

    Arrays with a single value can be written without brackets (`playlists: tv` instead of `playlists: ["tv"]`).

!!! warning "Strict Validation"

    Configuration files are validated strictly: unknown fields cause an error. If you use YAML anchors, prefix the
    anchor key with a dot (e.g. `.my_anchor`) so it isn't treated as an unknown field.

## Root Level Configuration

| Field            | Type                                         | Description                                                       |
| ---------------- | -------------------------------------------- | ----------------------------------------------------------------- |
| `server`         | [`Server`](./config/server.md)               | Server configuration including listening addresses and public URL |
| `url_generator`  | [`URL Generator`](./config/url_generator.md) | URL generation and encryption configuration                       |
| `logs`           | [`Logs`](./config/logs.md)                   | Logging configuration                                             |
| `proxy`          | [`Proxy`](./config/proxy.md)                 | Stream proxy configuration for remuxing with ffmpeg               |
| `playout`        | [`Playout`](./config/playout.md)             | Settings for channels generated from local media (24/7 live TV)   |
| `playlists`      | [`Playlists`](./config/playlists.md)         | Array of playlist definitions with sources and/or channels        |
| `epgs`           | [`EPGs`](./config/epgs.md)                   | Array of EPG definitions with sources                             |
| `channel_rules`  | [`Channel Rules`](./config/rules/index.md)   | Global channel processing rules (applied to all channels)         |
| `playlist_rules` | [`Playlist Rules`](./config/rules/index.md)  | Global playlist processing rules (applied after channel rules)    |
| `clients`        | [`Clients`](./config/clients.md)             | Array of IPTV client definitions with individual settings         |
