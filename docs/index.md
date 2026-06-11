---
hide:
  - navigation
  - toc
---

<div style="max-width: 850px; margin: 0 auto;" markdown>

# Majmun

<div style="display: flex; align-items: center; gap: 1em; flex-wrap: wrap;">
  <img src="assets/logo-tv.svg" alt="logo" width="100"/>
  <div style="flex: 1; min-width: 250px;">
    <strong>A minimal, functional IPTV gateway for your home TVs.</strong><br/>
    Transform and proxy your M3U playlists, EPG, and video streams through a single entry point. Configure playlists exactly how each client needs them with a flexible YAML configuration.
  </div>
</div>

<style>
@media (max-width: 500px) {
  div[style*="flex-wrap"] {
    flex-direction: column;
    text-align: center;
  }
}
</style>

---

![Diagram](./assets/diagram.svg)

### :material-lightbulb-outline: How It Works

You point Majmun at your provider's M3U playlists and EPG guides. It fetches them, applies your transformations
(rename, filter, sort, deduplicate), and serves each TV a personal playlist URL. When proxying is enabled, video
streams also flow through Majmun: it fetches each upstream stream once and serves it to any number of TVs, with
FFmpeg (or any command you configure) doing the stream processing.

### :material-playlist-music: Features

- Combine multiple playlist and EPG sources into one
- Transform channels: add or remove fields, set values using the full power of Go templates
- Transform playlists: filter, sort, merge, or remove duplicates
- Configure proxies and limits at global, playlist, or client level
- Serve one upstream stream to multiple TVs at once - the provider sees a single connection
- Show custom error videos for limits, stream failures, and expired links
- Cache playlists, guides, and logos on disk with configurable retention
- Generate 24/7 channels from your media files - a lightweight alternative to [ErsatzTV](https://ersatztv.org/) and [Tunarr](https://tunarr.com/)

Everything runs as a single binary - no database required. Each device gets its own playlist and EPG URLs, set up
once: upstream providers, channels, and rules can change server-side without touching the device again.

---

Why **Majmun**? It means 🐒 in Balkan languages. It's just for fun and to avoid interfering with other apps
