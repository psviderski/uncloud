# Metrics

Since Uncloud version 0.20 it exports [Prometheus](https://prometheus.io) metrics. The metrics are exposed on
port 51090. This port is exposed _in_ the cluster network, this makes it possible for a Prometheus service
running inside the cluster to scrape it.

The following metrics are exported by `uncloudd`. It is expected that future releases add more.

| Metric name                   | Labels               | Description                                                                                                 |
| ----------------------------- | -------------------- | ----------------------------------------------------------------------------------------------------------- |
| `uncloud_uncloudd_build_info` | `version`            | The `version` label holds the version                                                                       |
| `uncloud_dns_query_total`     | `internal`, `status` | The `internal` label is "true" for internal dns queries, and "false" otherwise, status holds "err", or "ok" |

Assuming Caddy is a global deployment and runs on all machines where `uncloudd` runs you can use the following
to have Prometheus scrape it.

```yaml title="prometheus.yaml"
scrape_configs:
  - job_name: uncloud
    dns_sd_configs:
      - names: ["m.internal"]
        type: A
        port: 51090
```

TODO(miek): this requires PR-359 to be merged.

# Caddy metrics

It is also possible to get [metrics](https://caddyserver.com/docs/metrics) from Caddy, for this you need to
enable them in the [global `x-caddy`](../3-concepts/2-ingress/3-managing-caddy.md) configuration.

```caddyfile
# Global options.
{
    debug

    metrics {
        per_host
    }
}
```

But this only exposes the metrics on localhost, to be able to scrape them from Prometheus you need a tiny
extra service who's only job it is to inject a snippet into Caddy's config.

```yaml title="compose.yaml"
services:
  caddy-metrics:
    image: caddy:latest
    x-caddy: |
      http://:2019 {
          handle {
              metrics
          }
      }
```

When this service is running you can scrape Caddy's metrics on port 2019.

```yaml title="prometheus.yaml"
scrape_configs:
  - job_name: caddy
    dns_sd_configs:
      - names: ["caddy.internal"]
        type: A
        port: 2019
```
