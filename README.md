# Async Proxy
> Fast-response middleware that proxies requests asynchronously

This service Helps proxying requests with the speed Golang can provide. It immediately returns the response, so the sender thinks request was handled, and proxies it to other side asynchronously.

## How it works

```
POST /some_endpoint -> asyncproxy
             200 OK <- asyncproxy
                       asyncproxy -> POST ${PROXY_REMOTE_URL}/some_endpoint
```

## Configuration

Configuration file is `config.yaml`. It contains the fine defaults for some settings. But all the settings can be owerwritten with ENV variables. This setting -

```yaml
proxy:
  remote_url: http://localhost:5000
```

- can be overwritten with ENV:

```bash
PROXY_REMOTE_URL=http://localhost:5001
```

### Configuration options

| Setting                | Description
| ----                   | ---- |
|`server.bind`           | binding port for the HTTP server. |
|`server.response_status`| the return code for incoming requests. |
|`proxy.remote_url`      | base URL for the destination server (must contain http(s):// prefix) |
|`proxy.num_clients`     | numbe of concurrent clients (goroutines) proxying the requests. The more the number is - the more file descriptors will be borrowed by process. That's why it should be limited. |
|`proxy.request.timeout` | the timeout in seconds each requests will be waiting for the response. This controls for how long one file descriptor is borrowed by process. |
|`proxy.request.method`  |the method that is to be proxied. Other requests are just dropped. |
|`metrics.path`          | URI for the Prometheus metrics exported. |