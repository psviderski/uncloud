# Publishing services

Publishing service ports makes your services available outside the cluster. This means your services can be accessed
from the internet or local network, depending on your setup.

You can publish service ports in three ways:

- Using the `-p/--publish` flag with `uc run`.
- Using the `x-ports` extension in a Compose file with `uc deploy`.
- Using the `--caddyfile` flag with `uc run` or `x-caddy` extension in a Compose file for custom Caddy configuration.

For example, run a service with container port 8000 exposed as https://app.example.com via Caddy reverse proxy:

```shell
uc run -p app.example.com:8000/https app:latest
```

```
[+] Running service app-mwng (replicated mode) 1/1
 ✔ Container app-mwng-6lub on machine-fnr9  Started

app-mwng endpoints:
 • https://app.example.com → :8000
```

Create an `A` record in your DNS provider (Cloudflare, Namecheap, etc.) pointing `app.example.com` to the public IP
address or your machine(s). Once DNS is propagated and Caddy obtains a TLS certificate, you can access your service
securely over HTTPS.

## Ingress vs host mode

**HTTP/HTTPS** ports are exposed via Caddy using the following format for the `-p/--publish` flag and `x-ports`
extension:

```
[hostname:]container_port[/protocol]
```

- `hostname` (optional): The domain name to use for accessing the service. If omitted and a cluster domain is reserved,
  `<service-name>.<cluster-domain>` is used.
- `container_port`: The port number within the container that's listening for traffic.
- `protocol` (optional): `http` or `https` (default: `https`)

**TCP/UDP** ports can only be exposed in host mode, which binds the container port directly to the host machine's
network interface(s). This is useful for non-HTTP services that need direct port access (bypasses Caddy):

```
[host_ip:]host_port:container_port[/protocol]@host
```

- `host_ip` (optional): The IP address on the host to bind to. If omitted, binds to all interfaces.
- `host_port`: The port number on the host to bind to.
- `container_port`: The port number within the container that's listening for traffic.
- `protocol` (optional): `tcp` or `udp` (default: `tcp`)

| Port value                   | Description                                                                          |
|------------------------------|--------------------------------------------------------------------------------------|
| `8000/http`                  | Publish port 8000 as HTTP via Caddy using hostname `<service-name>.<cluster-domain>` |
| `app.example.com:8080/https` | Publish port 8080 as HTTPS via Caddy using hostname `app.example.com`                |
| `127.0.0.1:5432:5432@host`   | Bind TCP port 5432 to host port 5432 on loopback interface only                      |
| `53:5353/udp@host`           | Bind UDP port 5353 to host port 53 on all network interfaces                         |

:::warning

Do not publish internal-only services like databases unless absolutely necessary. You only need to publish ports for
services that should be accessible from outside the cluster. Services within the cluster can communicate with each other
by their DNS names `service-name` or `service-name.internal` without publishing ports.

:::

## Using Compose

Use the `x-ports` extension in a Compose file to publish service ports:

```yaml title="compose.yaml"
services:
  app:
    image: app:latest
    x-ports:
      - example.com:8000/https
      - www.example.com:8000/https  # The same port can be published with multiple hostnames
      - api.domain.tld:9000/https   # Another port can be published with a different hostname
```

## Custom Caddy configuration

For advanced routing and behavior, use `x-caddy` instead of `x-ports`. It allows you to provide custom Caddy
configuration for a service in [Caddyfile](https://caddyserver.com/docs/caddyfile) format.

```yaml title="compose.yaml"
services:
  app:
    image: app:latest
    x-caddy: |
      www.example.com {
          redir https://example.com{uri} permanent
      }

      example.com {
          basic_auth /admin/* {
              admin $2a$14$...  # bcrypt hash
          }

          header /static/* Cache-Control max-age=604800
          reverse_proxy {{upstreams 8000}} {
              import common_proxy
          }
          log
      }
```

You can inline the Caddyfile or load it from a file: `x-caddy: ./Caddyfile`. When using a file, the path is relative to
the Compose file location. See the [Caddy documentation](https://caddyserver.com/docs/caddyfile) for syntax and
features.

:::info note

You cannot use `x-caddy` with `http` or `https` ports in `x-ports`. `tcp` and `udp` ports in host mode are allowed
though.

:::

Use it when you need:

- Custom routing rules (different paths, redirects, rewrites, multiple services on one domain).
- Custom headers, authentication, or caching.
- Custom load balancing strategies and options.
- Request and response manipulation.
- Advanced TLS settings.
- Other Caddy features and plugins.

See [Deploying or updating Caddy](3-managing-caddy.md#deploying-or-updating-caddy) for details on deploying Caddy with a
custom global configuration.

### Templates

`x-caddy` configs are processed as [Go templates](https://pkg.go.dev/text/template), allowing you to use dynamic values.
The following functions and variables are available:

| Template                              | Description                                                                                   |
|---------------------------------------|-----------------------------------------------------------------------------------------------|
| `{{upstreams [service-name] [port]}}` | A space-separated list of healthy container IPs for the current or specified service and port |
| `{{.Name}}`                           | The name of the service the config belongs to                                                 |
| `{{.Upstreams}}`                      | A map of all service names to their healthy container IPs                                     |

The templates are automatically re-rendered and Caddy is reloaded when service containers start/stop or health status
changes.

**Examples:**

1. Current service upstreams, default port:
   ```caddyfile
   reverse_proxy {{upstreams}}
   ```
   ↓

   ```caddyfile
   reverse_proxy 10.210.1.3 10.210.2.5
   ```
2. Current service upstreams, port 8000:
   ```caddyfile
   reverse_proxy {{upstreams 8000}}
   ```
   ↓

   ```caddyfile
   reverse_proxy 10.210.1.3:8000 10.210.2.5:8000
   ```
3. Current service upstreams with `https` scheme:
   ```caddyfile
   reverse_proxy {{- range $ip := index .Upstreams .Name}} https://{{$ip}}{{end}}
   ```
   ↓

   ```caddyfile
   reverse_proxy https://10.210.1.3 https://10.210.2.5
   ```
4. `api` service upstreams, port 9000:
   ```caddyfile
   handle_path /api/* {
       reverse_proxy {{upstreams "api" 9000}}
   }
   ```
   ↓

   ```caddyfile
   handle_path /api/* {
       reverse_proxy 10.210.2.2:9000 10.210.1.7:9000 10.210.2.3:9000
   }
   ```

### Verifying Caddy config

Use `uc caddy config` to view the complete generated Caddyfile served by the `caddy` service. This is useful for
debugging and verifying your `x-caddy` configs.

Example output:

```caddyfile
# Caddyfile autogenerated by Uncloud (DO NOT EDIT): 2025-12-20T22:43:56Z
# Automatically updated on service or health status changes.
# Docs: https://uncloud.run/docs/concepts/ingress/overview

# User-defined global config from service 'caddy'.
*.example.com {
	tls {
		dns cloudflare {env.CLOUDFLARE_API_TOKEN}
	}
	respond "No host matched" 404
}

# Health check endpoint to verify Caddy reachability on this machine.
http:// {
	handle /.uncloud-verify {
		respond "a369b9388812f9557feef6a0f5b46f2e" 200
	}
	log
}

(common_proxy) {
	# Retry failed requests up to lb_retries times against other available upstreams.
	lb_retries 3
	# Upstreams are marked unhealthy for fail_duration after a failed request (passive health checking).
	fail_duration 30s
}

# Sites generated from service ports.

https://app.example.com {
	reverse_proxy 10.210.1.3:8000 10.210.2.5:8000 {
		import common_proxy
	}
	log
}

https://api.example.com {
	reverse_proxy 10.210.2.2:9000 10.210.1.7:9000 10.210.2.3:9000 {
		import common_proxy
	}
	log
}

# User-defined config for service 'web'.
www.example.com {
    redir https://example.com{uri} permanent
}

example.com {
    reverse_proxy 10.210.0.3:8000 {
        import common_proxy
    }
    log
}

# Skipped invalid user-defined configs:
# - service 'duplicate-hostname': validation failed: adapting config using caddyfile adapter: ambiguous site definition: example.com
# - service 'invalid': validation failed: adapting config using caddyfile adapter: Caddyfile:61: unrecognized directive: invalid_directive
```

The generated config combines:

- Global Caddy configuration (`x-caddy` from the `caddy` service).
  See [Deploying or updating Caddy](3-managing-caddy.md#deploying-or-updating-caddy) for details.
- Auto-generated configs from published service ports (`x-ports`).
- Custom Caddy configs from services (`x-caddy`).
- Skipped invalid configs with error messages as comments.

:::warning important

Custom Caddy configs from different services must not conflict (all services must use unique hostnames).
See [Multiple services on one domain](#multiple-services-on-one-domain) for an example of how to share one hostname
between multiple services.

Conflicting or invalid configs are detected using [caddy adapt](https://caddyserver.com/docs/command-line#caddy-adapt)
command and skipped. However, some errors could still break the entire config so Caddy will fail to load it. Check the
`caddy` service logs to troubleshoot.

:::

### Common use cases

#### Redirects

Publish a service on `example.com` and redirect requests from `www.example.com` to `example.com`:

<Tabs>
<TabItem value="compose.yaml">

```yaml
services:
  app:
    image: app:latest
    x-caddy: ./Caddyfile
```

</TabItem>
<TabItem value="Caddyfile">

```caddyfile
www.example.com {
    redir https://example.com{uri} permanent
}

example.com {
    reverse_proxy {{upstreams 8000}} {
        import common_proxy
    }
    log
}
```

</TabItem>
</Tabs>

#### Multiple services on one domain

You can publish multiple services on the same hostname by using different paths for each service. For example, route
`/` to the web service and `/api` to the API service:

<Tabs>
<TabItem value="compose.yaml">

```yaml
services:
  api:
    image: api:latest
  web:
    image: web:latest
    # Make sure only one service defines a Caddy config for the hostname.
    x-caddy: ./Caddyfile
```

</TabItem>
<TabItem value="Caddyfile">

```caddyfile
example.com {
    handle_path /api/* {
        reverse_proxy {{upstreams "api" 9000}} {
            import common_proxy
        }
    }

    reverse_proxy {{upstreams}} {
        import common_proxy
    }

    log
}
```

</TabItem>
</Tabs>
