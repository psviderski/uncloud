# uc build

Build services from a Compose file.

## Synopsis

Build images for services from a Compose file using local Docker.

By default, built images remain on the local Docker host. Use --push to upload them
to cluster machines or --push-registry to upload them to external registries.

```
uc build [FLAGS] [SERVICE...] [flags]
```

## Examples

```
  # Build all services that have a build section in compose.yaml.
  uc build

  # Build specific services that have a build section.
  uc build web api

  # Build services and push images to all cluster machines or service x-machines if specified.
  uc build --push

  # Build services and push images to specific machines.
  uc build --push -m machine1,machine2

  # Build services and push images to external registries (e.g., Docker Hub).
  uc build --push-registry

  # Build services with build arguments, pull newer base images before building, and don't use cache.
  uc build --build-arg NODE_VERSION=24 --build-arg ENV=production --no-cache --pull
```

## Options

```
      --build-arg stringArray   Set a build-time variable for services. Used in Dockerfiles that declare the variable with ARG.
                                Can be specified multiple times. Format: --build-arg VAR=VALUE
      --check                   Check the build configuration for services without building them.
      --deps                    Also build services declared as dependencies of the selected services.
  -f, --file strings            One or more Compose files to build. (default compose.yaml)
  -h, --help                    help for build
  -m, --machine strings         Machine names or IDs to push the built images to (requires --push).
                                Can be specified multiple times or as a comma-separated list. (default is all machines or x-machines)
      --no-cache                Do not use cache when building images.
  -p, --profile strings         One or more Compose profiles to enable.
      --pull                    Always attempt to pull newer versions of base images before building.
      --push                    Upload the built images to cluster machines after building.
                                Use --machine to specify which machines. (default is all machines)
      --push-registry           Upload the built images to external registries (e.g., Docker Hub) after building.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.

