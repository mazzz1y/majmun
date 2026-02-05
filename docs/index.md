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

### :material-playlist-music: Features

* Use multiple stream sources
* Transform channels: add or remove fields, set values using the full power of Go templates
* Transform playlists: filter, sort, merge, or remove duplicates
* Configure proxies and limits at global, playlist, or client level
* Demultiplex single streams to multiple TVs
* Generate custom errors for limits, stream failures, and expired links
* Proxy and cache all connections to 3rd party services, cache with configurable retention to all static assets

Majmun acts as a lightweight wrapper over FFmpeg (or other stream processors).

Everything operates statelessly with a single component - no database required. Clients receive a personal encrypted link with channel properties and a TTL, allowing them to access Majmun regardless of any server-side changes or playlist's transform.

---

Why **Majmun**? It means 🐒 in Balkan languages. It's just for fun and to avoid interfering with other apps