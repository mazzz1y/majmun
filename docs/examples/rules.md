<div style="max-width: 850px; margin: 0 auto;" markdown>

# Rules

Rewrite, deduplicate, and sort channels — globally or per client. This config tags sports channels, then keeps
different quality tiers for different clients.

```yaml
server:
  listen_addr: ":8080"
  public_url: "http://localhost:8080"

url_generator:
  secret: "your-secret-key"

playlists:
  - name: tv
    sources: "https://provider.com/tv.m3u8"
  - name: sports
    sources: "https://provider.com/sports.m3u8"

channel_rules:
  # Tag sports channels by playlist or name pattern
  - set_field:
      selector: attr/group-title
      template: "Sports"
      condition:
        or:
          - playlists: ["sports"]
          - patterns: [".*ESPN.*", ".*Fox Sports.*", ".*Sky Sports.*"]

playlist_rules:
  # Living-room TV: prefer the highest available quality
  - remove_duplicates:
      patterns: ["4K", "FHD", "HD", "SD"]
      condition:
        clients: ["living-room"]

  # Mobile: prefer the lowest quality to save bandwidth
  - remove_duplicates:
      patterns: ["SD", "HD", "FHD", "4K"]
      condition:
        clients: ["mobile"]

clients:
  - name: "living-room"
    secret: "lr-secret"
    playlists: ["tv", "sports"]
  - name: "mobile"
    secret: "mb-secret"
    playlists: ["tv"]
```

See also: [Rules](../config/rules/index.md), [Set Field](../config/rules/channel_rules/set_field.md),
[Remove Duplicates](../config/rules/playlist_rules/remove_duplicates.md), [Condition](../config/shared/condition.md).

</div>
