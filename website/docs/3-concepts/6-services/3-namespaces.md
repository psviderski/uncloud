# Namespaces

Namespaces provide network isolation between groups of services running inside a cluster. Each namespace has its own
service discovery scope and traffic between namespaces is filtered at the bridge so that workloads in one namespace
cannot talk to workloads in another unless traffic goes through an explicitly published ingress.

## When to use namespaces
- Separate environments such as `staging` and `prod` on the same machines without risking accidental cross-talk.
- Multi-tenant clusters where each tenant should only see its own internal services.
- Isolating noisy or experimental workloads from core services while still sharing the same hosts.

If you do nothing, services run in the `default` namespace and can reach each other as before.

## Isolation behavior
- **DNS scope:** The embedded DNS server looks at the source IP of the request and only returns records for services in
  the same namespace. Queries from a service in the `prod` namespace will never receive IPs for `staging` services.
  Host-level lookups (e.g. `nslookup ...` from the machine) use the `default` namespace.
- **Network filtering:** Containers are grouped into per-namespace ipsets and iptables rules drop traffic that attempts
  to cross namespaces on the Uncloud bridge. Published ports or HTTPS ingress still behave normally and can be shared
  across namespaces if you expose them.
- **Defaults:** Any service without an explicit namespace is assigned to `default`. Namespace names must follow DNS label
  rules: lowercase alphanumeric with optional hyphens, 1–63 characters.

## Setting the namespace in Compose
Use the `x-namespace` extension on a service to place it into a namespace:

```yaml
services:
  api:
    image: ghcr.io/acme/api:latest
    x-namespace: prod
  worker:
    image: ghcr.io/acme/worker:latest
    x-namespace: prod
```

You can override every service's namespace at deploy time:

```bash
uc deploy --namespace staging
```

This is useful for promoting the same compose file into different isolated environments without editing YAML.
