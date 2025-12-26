# uc machine init

Initialise a new cluster with a remote machine as the first member.

## Synopsis

Initialise a new cluster by setting up a remote machine as the first member.
This command creates a new context in your Uncloud config to manage the cluster.

Connection methods:
  ssh://user@host       - Use built-in SSH library (default, no prefix required)
  ssh+cli://user@host   - Use system SSH command (supports ProxyJump, SSH config)

```
uc machine init [schema://]USER@HOST[:PORT] [flags]
```

## Examples

```
  # Initialise a new cluster with default settings.
  uc machine init root@<your-server-ip>

  # Initialise with a context name 'prod' in the Uncloud config (~/.config/uncloud/config.yaml) and machine name 'vps1'.
  uc machine init root@<your-server-ip> -c prod -n vps1

  # Initialise with a non-root user and custom SSH port and key.
  uc machine init ubuntu@<your-server-ip>:2222 -i ~/.ssh/mykey

  # Initialise without Caddy (no reverse proxy) and without an automatically managed domain name (xxxxxx.uncld.dev).
  # You can deploy Caddy with 'uc caddy deploy' and reserve a domain with 'uc dns reserve' later.
  uc machine init root@<your-server-ip> --no-caddy --no-dns
```

## Options

```
  -c, --context string        Name of the new context to be created in the Uncloud config to manage the cluster. (default "default")
      --dns-endpoint string   API endpoint for the Uncloud DNS service. (default "https://dns.uncloud.run/v1")
  -h, --help                  help for init
  -n, --name string           Assign a name to the machine.
      --network string        IPv4 network CIDR to use for machines and services. (default "10.210.0.0/16")
      --no-caddy              Don't deploy Caddy reverse proxy service to the machine. You can deploy it later with 'uc caddy deploy'.
      --no-dns                Don't reserve a cluster domain in Uncloud DNS. You can reserve it later with 'uc dns reserve'.
      --no-install            Skip installation of Docker, Uncloud daemon, and dependencies on the machine. Assumes they're already installed and running.
      --public-ip string      Public IP address of the machine for ingress configuration. Use 'auto' for automatic detection, blank '' or 'none' to disable ingress on this machine, or specify an IP address. (default "auto")
  -i, --ssh-key string        Path to SSH private key for remote login (if not already added to SSH agent). (default "~/.ssh/id_ed25519")
      --version string        Version of the Uncloud daemon to install on the machine. (default "latest")
  -y, --yes                   Auto-confirm prompts (e.g., resetting an already initialised machine).
                              Should be explicitly set when running non-interactively, e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in the cluster.

