# uc machine rtt

Show round-trip times between machines.

## Synopsis

Show round-trip times between machines.

Round-trip time statistics are collected from the Corrosion gossip protocol
and represent the average of recent RTT samples between each pair of machines
in the cluster. The values shown include the average RTT and standard deviation
for each machine-to-machine connection.

```
uc machine rtt [flags]
```

## Options

```
  -h, --help   help for rtt
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in the cluster.

