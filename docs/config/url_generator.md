# URL Generator

When proxying is enabled, the playlists Majmun serves don't contain the provider's original URLs. Every stream link
and every proxied asset link (channel logos, for example) is replaced with an encrypted token pointing at your
`public_url`. The URL generator controls how these tokens are created and how long they stay valid.

## YAML Structure

```yaml
url_generator:
  secret: ""
  stream_ttl: ""
  file_ttl: ""
```

## Fields

| Field        | Type                             | Required | Default             | Description                                          |
| ------------ | -------------------------------- | -------- | ------------------- | ---------------------------------------------------- |
| `secret`     | `string`                         | Yes      | ``                  | Server-side salt for URL encryption                  |
| `stream_ttl` | [`duration`](shared/duration.md) | No       | `30 days`           | Time-to-live for stream links (0 = no expiration)    |
| `file_ttl`   | [`duration`](shared/duration.md) | No       | `0 (no expiration)` | Time-to-live for asset links (0 = no expiration)     |

!!! note "Secret Key"

    The `secret` is combined with each client's own secret to encrypt URLs. Changing it invalidates every link ever
    generated — all TVs will need to refresh their playlists.

!!! note "How TTL behaves"

    Links carry their expiration inside the encrypted token, so each playlist fetch produces fresh links valid for
    the full TTL. A TV that refreshes its playlist regularly never sees an expired link; a stale playlist serves
    the [`link_expired`](proxy/error.md) error video once `stream_ttl` passes. `file_ttl` defaults to `0` (never
    expires) since assets like logos aren't sensitive.

## URL Generation

The URL generator creates encrypted URLs in the following format:

```
{public_url}/{encrypted_token}{extension}
```

Where:

- `{encrypted_token}` contains encrypted stream information and expiration time
- `{extension}` is determined by the content type (`.ts` for streams, original extension for files)
