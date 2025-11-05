# uc image push

Upload a local Docker image to the cluster.

## Synopsis

Upload a local Docker image to the cluster transferring only the missing layers.
The image is uploaded to the machine which CLI is connected to (default) or the specified machine(s).

```
uc image push IMAGE [flags]
```

## Examples

```
  # Push image to the machine the CLI is connected to.
  uc image push myapp:latest

  # Push image to specific machine.
  uc image push myapp:latest -m machine1

  # Push image to multiple machines.
  uc image push myapp:latest -m machine1,machine2,machine3

  # Push image to all machines in the cluster.
  uc image push myapp:latest -m all

  # Push a specific platform of a multi-platform image.
  uc image push myapp:latest --platform linux/amd64
```

## Options

```
  -c, --context string    Name of the cluster context. (default is the current context)
  -h, --help              help for push
  -m, --machine strings   Machine names or IDs to push the image to. Can be specified multiple times or as a comma-separated list. (default is all machines)
      --platform string   Push a specific platform of a multi-platform image (e.g., linux/amd64, linux/arm64).
                          Local Docker must be configured to use containerd image store to support multi-platform images.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc image](uc_image.md)	 - Manage images on machines in the cluster.

