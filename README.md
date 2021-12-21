[![CircleCI](https://circleci.com/gh/evilmartians/asyncproxy/tree/main.svg?style=svg)](https://circleci.com/gh/evilmartians/asyncproxy/tree/main)

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

| Setting                 | Description
| ----                    | ---- |
|`server.bind`            | binding port for the HTTP server. |
|`server.response_status` | the return code for incoming requests. |
|`server.shutdown_timeout`| the time you give the service to complete the requests and gracefully shutdown |
|`metrics.path`           | URI for the Prometheus metrics exported. |
|`metrics.bind`           | binding port for the metrics server. |
|`proxy.remote_url`       | base URL for the destination server (must contain http(s):// prefix) |
|`proxy.request_timeout`  | the time each requests will be waiting for the response. This controls for how long one file descriptor is borrowed by process. |
|`proxy.num_clients`      | number of concurrent clients (goroutines) proxying the requests. Limits the number of open file descriptors. |
|`queue.workers`          | number of workers processing the queue |
|`queue.handle_per_second`| Limit for fetching requests from DB |
|`queue.max_retries`      | Maximum number of attempts to resend the request if the previous wasn't successful |
|`db.connection_string`   | database connection string |
|`db.max_connections`     | max open connections to database allowed |
|`db.use_index`           | whether to query database with index scan or seq scan |

### Configuration aspects

When setting `server.shutdown_timeout` and `queue.workers` consider the following: on shutdown the server waits for all workers to complete their requests. So, if proxying takes 0.1 seconds, it may take up to 30 seconds for 300 workers to gracefully shut down.