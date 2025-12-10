# uc machine

Manage machines in the cluster.

## Options

```
  -h, --help   help for machine
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
* [uc machine add](uc_machine_add.md)	 - Add a remote machine to a cluster.
* [uc machine init](uc_machine_init.md)	 - Initialise a new cluster with a remote machine as the first member.
* [uc machine ls](uc_machine_ls.md)	 - List machines in a cluster.
* [uc machine rename](uc_machine_rename.md)	 - Rename a machine in the cluster.
* [uc machine rm](uc_machine_rm.md)	 - Remove a machine from a cluster and reset it.
* [uc machine token](uc_machine_token.md)	 - Print the local machine's token for adding it to a cluster.
* [uc machine update](uc_machine_update.md)	 - Update machine configuration in the cluster.

