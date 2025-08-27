---
sidebar_label: Overview
---

# Ingress & HTTPS

Uncloud uses [Caddy](https://caddyserver.com/) as its reverse proxy to handle incoming traffic, provide automatic HTTPS
with [Let's Encrypt](https://letsencrypt.org/), and route requests to your services.

## How it works

Caddy runs as a global service `caddy` on every machine in your cluster, listening on the host ports 80 (HTTP) and 443
(HTTPS).

It's deployed during cluster initialisation (`uc machine init`) unless you use the `--no-caddy` flag.
See [Managing Caddy](3-managing-caddy.md) for deployment and customisation instructions.

When you [publish a service port](2-publishing-services.md), Uncloud automatically configures Caddy to:

1. Listen for requests on the specified hostname (domain name).
2. Automatically obtain and renew a TLS certificate from Let's Encrypt for HTTPS.
3. Route traffic to the **healthy** service container(s).
4. Load balance across healthy replicas if there are multiple.
