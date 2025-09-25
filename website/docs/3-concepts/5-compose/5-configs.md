# Configs

Uncloud supports [Compose configs](https://github.com/compose-spec/compose-spec/blob/main/08-configs.md) for managing configuration files in your services. Configs allow you to store non-sensitive configuration data separately from your container images and mount them into containers at runtime.
See also [Docker Compose documentation](https://docs.docker.com/reference/compose-file/configs/) for the same feature.

## Overview

Configs provide a way to:

- Store configuration files outside of container images
- Share configuration between multiple services
- Update configuration without rebuilding images
- Version control your configuration separately

## Defining Configs

Configs are defined in two places in your `compose.yaml`:

1. **Top-level `configs` section**: Define the config content
2. **Service-level `configs` section**: Mount configs into containers

## Top-level Configs

Define configs using either file-based or inline content:

### File-based Configs

Read configuration from a file on the (local/control) host where `uc deploy` is run:

```yaml
configs:
  nginx_config:
    file: ./nginx.conf
  app_config:
    file: ./config/app.properties
```

The file path is relative to the compose file location.

### Inline Configs

Define configuration content directly in the compose file:

```yaml
configs:
  app_config:
    content: |
      database_url=postgres://localhost:5432/myapp
      redis_url=redis://localhost:6379
      # Variable interpolation is supported
      log_level=${LOG_LEVEL:-info}
```

When using inline configs, [environment variable interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/) is supported so that you can customize configuration based on your deployment environment. Variables are resolved from the environment where `uc deploy` is executed.

## Service-level Config Mounts

Mount configs into containers using the long syntax:

```yaml
services:
  web:
    image: nginx:alpine
    configs:
      - source: nginx_config
        target: /etc/nginx/nginx.conf
        mode: 0644
      - source: app_config
        target: /app/config.properties
        uid: "1000"
        gid: "1000"
        mode: 0600
```

### Config Mount Options

| Option   | Description                                       | Default    |
| -------- | ------------------------------------------------- | ---------- |
| `source` | Name of the config (from top-level configs)       | Required   |
| `target` | Path where the config is mounted in the container | Required   |
| `mode`   | File permissions (octal format)                   | `0644`     |
| `uid`    | User ID that owns the file                        | Root user  |
| `gid`    | Group ID that owns the file                       | Root group |

## Complete Examples

### Example 1: Web Server with Custom Configuration

```yaml
services:
  web:
    image: nginx:alpine
    configs:
      - source: nginx_conf
        target: /etc/nginx/nginx.conf
    x-ports:
      - 80/https

configs:
  nginx_conf:
    file: ./nginx.conf
```

Create `nginx.conf` in the same directory as your compose file:

```nginx
events {
    worker_connections 1024;
}

http {
    server {
        listen 80;

        location / {
            return 200 'Hello from Uncloud!\n';
            add_header Content-Type text/plain;
        }
    }
}
```

### Example 2: Application with Multiple Config Files

```yaml
services:
  app:
    image: node:18-alpine
    command: ["node", "server.js"]
    configs:
      - source: app_config
        target: /app/config.json
        mode: 0644
      - source: database_config
        target: /app/database.json
        uid: "1000"
        gid: "1000"
        mode: 0600
    environment:
      NODE_ENV: production

configs:
  app_config:
    content: |
      {
        "port": 3000,
        "logLevel": "info",
        "features": {
          "analytics": true,
          "cache": true
        }
      }
  database_config:
    file: ./configs/database.json
```

## Implementation details

Here are the key characteristics of the configs feature implementation:

- **Client-side processing**: When you run `uc deploy`, the Uncloud CLI reads config files from your local machine and includes their content in the service specification.

- **Content transfer**: Config content (both file-based and inline) is sent to the Uncloud daemon via gRPC as part of the deployment request.

- **Container deployment**: During container creation, configs are copied inside the container.

- **File lifecycle**: Config files exist only for the lifetime of the container. When a container is removed, its config files are cleaned up automatically.

- **Per-container isolation**: Each container gets its own copy of config files.

- **Atomic updates**: Config changes require redeployment, ensuring consistency across all replicas.

## Best Practices

### Security Considerations

- **Sensitive Data**: Don't put secrets in configs. Use environment variables or external secret management
- **File Permissions**: Set appropriate `mode`, `uid`, and `gid` for sensitive config files
- **Version Control**: Be careful about committing sensitive configuration files to git

### Config Sharing

Configs can be shared across multiple services:

```yaml
services:
  web:
    image: nginx
    configs:
      - source: shared_config
        target: /etc/app/config.yaml

  api:
    image: myapi
    configs:
      - source: shared_config
        target: /app/config.yaml

configs:
  shared_config:
    content: |
      environment: production
      debug: false
```

## Limitations

- **External configs**: Not supported. All configs must be defined in the compose file
- **Short syntax**: Not yet supported. Use the long syntax with `source` and `target`
- **Config updates**: Changing config content requires redeployment to take effect

## Troubleshooting

### Config File Not Found

If you get an error about config file not found:

1. Check the file path is correct relative to the compose file
2. Ensure the file exists and is readable
3. Verify file permissions

### Permission Denied

If containers can't read config files:

1. Check the `mode` setting allows read access
2. Verify `uid` and `gid` match the container's user
3. Ensure the container user has permission to access the target directory

### Config Not Updating

If config changes don't take effect:

1. Run `uc deploy` to redeploy with new config content
2. Check that you're modifying the correct config file
3. Verify the config is properly mounted in the container with `docker exec <service> cat <config-path>` on the remote machine
