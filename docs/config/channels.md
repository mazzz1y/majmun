# Channels

Channels are 24/7 virtual linear TV channels generated from your local video files. Point a channel at a directory of
movies or episodes, and Majmun plays them back-to-back on a continuous looping schedule — like a classic TV channel.
It's a lightweight alternative to [ErsatzTV](https://ersatztv.org/) and [Tunarr](https://tunarr.com/): no separate
service, database, or web UI, just a few lines of YAML inside the playlist proxy you already run.

A channel is defined under a [playlist](./playlists.md)'s `channels` block and is delivered to any client that selects
that playlist. Each channel produces one entry in the client M3U8 and a matching EPG with one programme per file.

## How It Works

A channel scans its `sources`, probes each file's duration and title with `ffprobe`, and lays the files out on a
continuous looping timeline. The live position follows wall-clock time, so it survives restarts (the schedule is
persisted under [`state_dir`](../config.md)) and stays consistent for every viewer.

The video is produced by the [`proxy.playout`](./proxy/playout.md) command — one FFmpeg transcode per channel, shared
across all viewers.

The schedule is built at startup and rebuilt in the background when files change or `refresh_interval` elapses.
Probe results are cached, so unchanged files are never re-probed. Viewers are never interrupted: during a rebuild the
channel keeps serving the previous schedule, a deleted file is skipped on the fly, and a channel with no schedule yet
serves a placeholder stream.

## YAML Structure

```yaml
playlists:
  - name: local
    channels:
      - name: ""
        logo: ""
        fields:
          - selector: ""
            template: ""
        template_variables: []
        sources: []
        random_order: false
        extensions: []
        refresh_interval: 5m
        epg_duration: 1w
```

## Fields

| Field              | Type                                | Required | Description                                                                                                                                              |
| ------------------ | ----------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`             | `string`                            | Yes      | Display name shown in the playlist and guide. Must be unique within its playlist; may repeat across playlists.                                            |
| `logo`             | `string`                            | No       | Channel logo: an http(s) URL or a local file path.                                                                                                        |
| `fields`           | `[]field`                           | No       | Extra M3U fields to emit. Each entry sets one attr or tag via a [selector](./rules/selector.md) (`attr/<key>` or `tag/<key>`) and a Go `template`.        |
| `template_variables` | [`[]NameValue`](./shared.md#namevalue-object) | No | Extra variables for this channel's [playout](./proxy/playout.md) command. Override playout `template_variables` from the proxy cascade. |
| `sources`          | `[]string`                          | Yes      | Directories (scanned recursively) and/or individual video files.                                                                                         |
| `random_order`     | `bool`                              | No       | When `true`, files play in a stable shuffled order; otherwise they play in [episode order](#playback-order). Default `false`.                             |
| `extensions`       | `[]string`                          | No       | Video file extensions to include. Default: `mkv, mp4, avi, mov, m4v, ts, webm, mpg, mpeg, flv, wmv`.                                                      |
| `refresh_interval` | [`duration`](./shared.md#duration)  | No       | How often the schedule is rebuilt to pick up file changes. Default `5m`; `0` disables. The check is a cheap rescan — see [How It Works](#how-it-works).   |
| `epg_duration`     | [`duration`](./shared.md#duration)  | No       | How far into the future the EPG is generated. Default `1w`.                                                                                               |

### Channel Identity

The channel's stable id — used as the `tvg-id`, the XMLTV `<channel id>`, and the EPG join key — is derived
automatically by hashing the playlist and channel names; there is no id field to set. This keeps the id unique and
URL-safe even when the same `name` is reused in another playlist.

Hashes are technical identifiers only: schedule files under `state_dir` are named by the channel id, while logs and
metrics always show the human-readable channel and playlist names.

A channel belongs to its parent playlist: the [`proxy.playout`](./proxy/playout.md) command cascades global ➡
playlist (group channels that need the same command into one playlist), and [channel rules](./rules/index.md) with a
`condition.playlists` match the channel by its **parent playlist name**.

## Playback Order

Unless `random_order` is set, files play in episode order:

1. Files are grouped by directory; directories compare in natural order (`Season 2` before `Season 10`).
2. Within a directory, files with season/episode info play in `season, episode` order. The info comes from container
   tags (`episode_id`, `episode_sort`, `episode`) or, when absent, from the filename — `S01E05`, `1x05`, `ep05`,
   `Episode 5`, and `E05` style patterns are recognized.
3. Files without episode info compare by filename in natural order, so `ep2.mkv` plays before `ep10.mkv`.

## EPG Metadata

Each file becomes one EPG programme. Programme fields are taken from the file's container metadata tags when present:

- **title** — `title` tag, falling back to the file name without its extension.
- **description** (`<desc>`) — `description`, `synopsis`, `summary`, or `comment` tag.
- **category** (`<category>`) — `genre` tag.
- **date** (`<date>`) — `date`, `year`, or `date_released` tag, when it parses as a known date format (e.g. `2021`, `2021-06-08`).
- **episode** (`<episode-num>`) — `episode_id`/`episode_sort`/`episode` tag, either `SxxEyy` (e.g. `S01E03`) or a bare episode number.

Tag names are matched case-insensitively, so both MP4 (`title`, `genre`, ...) and Matroska (`TITLE`, `GENRE`, ...)
conventions work. Tags that are absent or in an unrecognized format are simply omitted from the guide.

## FFmpeg Transcode

A channel is transcoded by [`proxy.playout`](./proxy/playout.md). The default is a software
(libx264/aac) 24/7 loop that normalizes every source to a common resolution, frame rate, and audio layout, so sources
with different resolutions can be mixed freely — but software encoding is slow, and picking a command that fits your
hardware is your responsibility. See the [Playout](./proxy/playout.md) page for the input/output contract a command
must satisfy, the default command, and hardware-acceleration starting points.

## Examples

### Basic Channel

```yaml
state_dir: ./state

playlists:
  - name: local
    channels:
      - name: "Cartoons 24/7"
        fields:
          - selector: tag/EXTGRP
            template: "Kids"
        sources:
          - /media/cartoons

clients:
  - name: living-room
    secret: "living-room-secret"
    playlists: local
```

### Custom Fields

`fields` emits any M3U attr or tag, so the group (or anything else) can be expressed in whichever form a player
expects — `attr/group-title`, `tag/EXTGRP`, or both. Templates receive `.Channel` (`Name`, `Attrs`, `Tags`) and
`.Playlist` (`Name`, `IsProxied`), same as [`set_field`](./rules/channel_rules/set_field.md) rules.

```yaml
playlists:
  - name: local
    channels:
      - name: "Football"
        fields:
          - selector: attr/group-title
            template: "1337 v2"
          - selector: attr/tvg-shift
            template: "+2"
          - selector: tag/EXTGRP
            template: "{{ .Playlist.Name }}"
        sources: [/media/football]
```

### Multiple Sources and Random Order

Directories are scanned recursively; individual files can be mixed in. `random_order` shuffles playback with a stable,
persisted seed.

```yaml
playlists:
  - name: local
    channels:
      - name: "Movie Marathon"
        sources:
          - /media/movies
          - /media/extra/feature.mkv
        random_order: true
        extensions: [mkv, mp4]
```
