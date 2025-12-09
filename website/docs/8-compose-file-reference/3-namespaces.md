# Namespace extension (`x-namespace`)

Use the `x-namespace` extension on a service to place it into a network-isolated namespace. Services without this
extension use the `default` namespace.

```yaml
services:
  web:
    image: ghcr.io/acme/web:latest
    x-namespace: prod
```

Rules:
- Names must be lowercase alphanumeric with optional hyphens and 1–63 characters.
- The namespace is applied per service. Use `uc deploy --namespace <name>` to override the namespace for all services at
  deploy time (useful for promoting the same compose file to `staging`, `prod`, etc.).

Effect:
- Internal DNS only returns records for services in the same namespace as the caller.
- Traffic inside the Uncloud bridge is filtered so containers from different namespaces cannot communicate directly.
