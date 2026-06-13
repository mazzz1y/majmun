<div style="max-width: 850px; margin: 0 auto;" markdown>

# Examples

Ready-to-use configurations, one per feature. Each is a complete file you can copy, adjust the placeholders
(URLs, secrets, paths), and run.

!!! note "Exposed endpoints"

    Once running, each client gets:

    - `{public_url}/{client_secret}/playlist.m3u8`
    - `{public_url}/{client_secret}/epg.xml`
    - `{public_url}/{client_secret}/epg.xml.gz`

## Pages

- [Minimal](./minimal.md) — the smallest working setup: one playlist, one EPG, one client.
- [Proxy](./proxy.md) — restream through majmun with caching, custom headers, and error screens.
- [Playout](./playout.md) — 24/7 channels generated from local media, including hardware transcoding.
- [Rules](./rules.md) — rewrite, dedupe, and sort channels per client.
- [Full Setup](./full.md) — everything together: proxy, generated channels, rules, and multiple clients.

</div>
