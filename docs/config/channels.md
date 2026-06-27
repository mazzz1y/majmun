# Channels

Channels are 24/7 virtual linear TV channels generated from your local video files. Point a channel at a directory of
movies or episodes, and Majmun plays them back-to-back on a continuous looping schedule — like a classic TV channel.
It's a lightweight alternative to [ErsatzTV](https://ersatztv.org/) and [Tunarr](https://tunarr.com/): no separate
service, database, or web UI, just a few lines of YAML inside the playlist proxy you already run.

A channel is defined under a [playlist](./playlists.md)'s `channels` block and is delivered to any client that selects
that playlist. Each channel produces one entry in the client M3U8 and a matching EPG with one programme per file.

## How It Works

A channel scans its `sources`, lays the files out on a continuous looping timeline, and plays it as one shared
[`playout`](./playout.md) transcode. The live position follows wall-clock time, so it survives restarts and is the same
for every viewer.

### Picking Up File Changes

majmun re-scans the `sources` every [`refresh_interval`](./playout.md) (default `30m`) to notice files you've added or
removed. To avoid cutting off a show, a changed file set is not adopted the moment it's detected. Instead:

- **Added files** appear at the next [`schedule_swap_at`](./playout.md) time (local, default `04:00`), and only once
  the programme playing then finishes — so turnover always lands on a programme boundary.
- **Removed files** are also deferred to that swap, *unless* a removed file is next due to air before the swap lands
  (i.e. before the programme spanning the swap time finishes). A missing file would otherwise be skipped live, so that
  case triggers an immediate, controlled adoption of the new file set instead.

The first build is immediate (the channel starts right away), and if all sources disappear the channel keeps playing
its last good schedule.

### Catch-Up

Channels support rewind: a viewer can start watching from an earlier point in the guide instead of the live edge.
Players like TiviMate, Kodi, and IPTV-Smarters show a catch-up UI for any past programme and handle this automatically —
no setup needed.

Behind the scenes the player just adds a `?utc=<unix-time>` parameter to the channel URL, and the stream starts there
and plays forward toward live:

```
http://host/<channel-url>?utc=1718200000
```

How far back rewind goes is set automatically from your `epg_duration` (capped at one full loop of the channel's
content). Requesting a time in the future simply plays live.

## YAML Structure

```yaml
playlists:
  - name: local
    channels:
      - name: ""
        sources: []
        fields:
          - selector: ""
            template: ""
        playout: {} # per-channel override of any playout field (logo, schedule, command, ...)
```

## Fields

A channel itself has only three fields plus an optional `playout` override. Everything else — `logo`, `order`,
`extensions`, `refresh_interval`, `epg_duration`, `schedule_swap_at`, the transcode command and its
`template_variables` — lives in the [`playout`](./playout.md) block and cascades global ➡ playlist ➡ channel.

| Field     | Type                            | Required | Description                                                                                                                                        |
| --------- | ------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`    | `string`                        | Yes      | Display name shown in the playlist and guide. Must be unique within its playlist; may repeat across playlists.                                      |
| `sources` | `[]string`                      | Yes      | Directories (scanned recursively) and/or individual video files.                                                                                   |
| `fields`  | `[]field`                       | No       | Extra M3U fields to emit. Each entry sets one attr or tag via a [selector](./shared/selector.md) (`attr/<key>` or `tag/<key>`) and a Go `template`. |
| `playout` | [`Playout`](./playout.md)       | No       | Per-channel overrides of the playout config (logo, scheduling, transcode command/variables).                                                       |

A channel belongs to its parent playlist: the [`playout`](./playout.md) config cascades global ➡ playlist (group
channels that need the same command into one playlist) ➡ channel, and [channel rules](./rules/index.md) with a
`condition.playlists` match the channel by its **parent playlist name**.

## EPG Metadata

Each file becomes one EPG programme. Programme fields are taken from the file's container metadata tags when present:

- **title** — `title` tag, falling back to the file name without its extension.
- **description** (`<desc>`) — `description`, `synopsis`, `summary`, or `comment` tag.
- **category** (`<category>`) — `genre` tag.
- **date** (`<date>`) — `date`, `year`, or `date_released` tag, when it parses as a known date format (e.g. `2021`, `2021-06-08`).
- **episode** (`<episode-num>`) — `episode_id`/`episode_sort`/`episode` tag, either `SxxEyy` (e.g. `S01E03`) or a bare episode number.

Tag names are matched case-insensitively, so both MP4 (`title`, `genre`, ...) and Matroska (`TITLE`, `GENRE`, ...)
conventions work. Tags that are absent or in an unrecognized format are simply omitted from the guide.

The **title**, **description**, and **category** fields are Go templates you can override per channel — see
[Playout → Metadata Templates](./playout.md#metadata-templates) for how to customize them.

## FFmpeg Transcode

A channel is transcoded by [`playout`](./playout.md), which loops its sources into one continuous live stream. You
must provide the FFmpeg `command` — there is no default, since the right one depends on your hardware.

See the [Playout](./playout.md) page for what a command must do (normalize mixed sources, append to the live playlist,
pace at realtime) and [Examples → Playout](../examples/playout.md) for ready-to-use commands, including hardware
acceleration.

## Examples

### Basic Channel

```yaml
playout:
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

