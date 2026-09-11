# uc machine update

Update machine configuration in the cluster.

## Synopsis

Update machine configuration in the cluster.

Change the name, public IP address, WireGuard endpoints, or WireGuard listen port of an existing machine.
At least one flag must be specified to perform an update.

```
uc machine update MACHINE [flags]
```

## Examples

```
  # Rename a machine.
  uc machine update machine1 --name web-server

  # Set the public IP address of a machine.
  uc machine update machine1 --public-ip 203.0.113.10

  # Remove the public IP address from a machine.
  uc machine update machine1 --public-ip none

  # Update WireGuard endpoints for a machine.
  uc machine update machine1 --wg-endpoint 203.0.113.10 --wg-endpoint 192.168.1.5

  # Change the WireGuard listen port. Advertised endpoints using the old port are adjusted automatically.
  uc machine update machine1 --wg-port 51821

  # Change the listen port and set explicit endpoints (e.g. behind NAT with port forwarding).
  uc machine update machine1 --wg-port 51821 --wg-endpoint 203.0.113.10:9821

  # Update multiple properties at once.
  uc machine update machine1 --name web-server --public-ip 203.0.113.10
```

## Options

```
  -h, --help                  help for update
      --name string           New name for the machine
      --public-ip string      Public IP address of the machine for ingress configuration. Use 'none' or '' to remove the public IP.
      --wg-endpoint strings   WireGuard endpoint address that other machines in the cluster should use to establish WireGuard connections
                              to this machine. This doesn't change the port WireGuard listens on the machine (see --wg-port).
                              Format: IP, IP:PORT, IPv6, or [IPv6]:PORT. Default port is the value of --wg-port (or the current
                              listen port) if omitted.
                              Multiple endpoints can be specified by repeating the flag or using a comma-separated list.
      --wg-port int           UDP port WireGuard listens on for incoming connections from other machines.
                              Advertised endpoints using the old listen port are adjusted to the new port automatically
                              unless --wg-endpoint is provided. (default 51820)
  -y, --yes                   Auto-confirm the WireGuard listen port change prompt.
                              Should be explicitly set when running non-interactively, e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in the cluster.

