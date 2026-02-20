# Server

The server block is responsible for configuring HTTP servers.

## YAML Structure

```yaml
server:
  listen_addr: ""
  metrics_addr: ""
  public_url: ""
```

## Configuration Fields

| Field          | Type     | Required | Default                   | Description                                       |
| :------------- | :------- | :------- | :------------------------ | :------------------------------------------------ |
| `listen_addr`  | `string` | Yes      | `":8080"`                 | Address the gateway listens on                    |
| `public_url`   | `string` | Yes      | `"http://127.0.0.1:8080"` | Public URL of the gateway, used to generate links |
| `metrics_addr` | `string` | No       | `""`                      | Address for the metrics server, disabled if empty |
