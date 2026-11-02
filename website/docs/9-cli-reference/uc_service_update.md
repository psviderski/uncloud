# uc service update

Update a service in place without recreating its containers.

## Synopsis

Update service configuration in place without recreating its containers.

A CPU or memory limit can be changed but not removed. To remove a limit or change anything else,
redeploy the service with 'uc deploy'

```
uc service update SERVICE [flags]
```

## Examples

```
  # Change the CPU and memory limits.
  uc service update web --cpu 0.5 --memory 512m

  # Add or change a service label and remove another one.
  uc service update web --label-add tier=frontend --label-rm legacy
```

## Options

```
      --cpu decimal                Maximum number of CPU cores a service container can use.
                                   Fractional values are allowed: 0.5 for half a core or 2.25 for two and a quarter cores.
  -h, --help                       help for update
      --label-add stringArray      Add or change a service label. Can be specified multiple times.
                                   Format: key=value or just key for an empty value.
      --label-rm stringArray       Remove a service label by its key. Can be specified multiple times.
      --memory bytes               Maximum amount of memory a service container can use. Value is a positive integer with
                                   optional unit suffix (b, k, m, g). Default unit is bytes if no suffix specified.
                                   Examples: 1073741824, 1024m, 1g (all equal 1 gibibyte)
      --memory-reservation bytes   Minimum amount of memory reserved for a service container. Value is a positive integer with
                                   optional unit suffix (b, k, m, g). Default unit is bytes if no suffix specified.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc service](uc_service.md)	 - Manage services in the cluster.

