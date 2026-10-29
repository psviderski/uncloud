# uc image rm

Remove images from machines in the cluster.

## Synopsis

Remove images from all or selected machines in the cluster.

Images used by running or stopped containers cannot be removed. Unused tags can be removed even
when the same image is used under another tag.

An image name without a tag selects all its tags. Specify a tag, such as myapp:latest, to remove
only that tag.

Wildcard patterns are supported for image names and tags and should be quoted to prevent your
shell from expanding them. Removing by image ID attempts to remove each name for that image
independently. Failed removals do not prevent other removals.

```
uc image rm IMAGE [IMAGE...] [flags]
```

## Examples

```
  # Remove a tag from all machines after confirming the plan.
  uc image rm myapp:old

  # Remove all tags for an image name from all machines.
  uc image rm myapp

  # Remove matching tags from two machines without confirmation.
  uc image rm 'myapp:1.*' 'otherapp:*' -m machine1,machine2 --yes

  # Remove all names for an image by ID from all machines.
  uc image rm ee8fed0e9618
```

## Options

```
  -h, --help              help for rm
  -m, --machine strings   Machine names or IDs to remove images from. Can be specified multiple times or as a comma-separated list.
                          If not specified, the matching images will be removed from all machines.
  -y, --yes               Auto-confirm the removal plan. Should be explicitly set when running non-interactively,
                          e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc image](uc_image.md)	 - Manage images on machines in the cluster.

