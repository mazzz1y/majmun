# Duration

Duration values support the following units:

- `s` - seconds
- `m` - minutes
- `h` - hours
- `d` - days (24 hours)
- `w` - weeks (7 days)
- `M` - months (30 days)
- `y` - years (365 days)

Examples: `30s`, `5m`, `2h`, `1d`, `2w`

A bare `0` (no unit) is also accepted; fields document what `0` means for them (usually "disabled" or "never
expires").
