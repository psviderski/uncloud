# Deploy demo app

In this guide, we'll deploy [Excalidraw](https://excalidraw.com), a popular sketching and diagramming tool, to your
Linux server. You'll learn the **basics of Uncloud** and see how simple it is to **run web apps** on your own
infrastructure with secure internet access.

:::info NOTE

To give you a chance to play with Uncloud without even leaving your browser or needing your own servers, we're providing
interactive tutorials and playgrounds on the [iximiuz Labs](https://labs.iximiuz.com/) platform.

You can follow [this tutorial](https://labs.iximiuz.com/tutorials/uncloud-create-cluster-ebebf72b) which walks you
through creating a new cluster with two machines and then deploying a simple web service to it.

You can also launch the [Uncloud playground](https://labs.iximiuz.com/playgrounds/uncloud-cluster-64523f7c) where you
can play with an already initialised Uncloud cluster.
:::

## Prerequisites

Before you begin, you'll need:

- **Uncloud CLI** [installed](1-install-cli.md) on your local machine
- A **Ubuntu or Debian server** with **public IP address** and **SSH access** using a **private key** (as `root` or a
  user with **passwordless** `sudo` privileges).

:::tip Need a server?

A small Virtual Private Server (VPS) or dedicated server from providers like [Hetzner](https://www.hetzner.com) or
[DigitalOcean](https://www.digitalocean.com) is a great choice for learning Uncloud and running lightweight services. We
recommend using a freshly installed server as existing services on ports 80 and 443 can cause conflicts.

**Minimum requirements:** 1 vCPU, 512 MB RAM, Ubuntu 22.04 or Debian 11, AMD64 (recommended) or ARM64 architecture.
Other Linux distributions may work, but haven't been tested yet.

:::

## Set up your server

First, let's turn your server into an Uncloud **machine**. This simply means setting it up so you can deploy and manage
services on it using `uc`.

```shell
uc machine init root@<your-server-ip>
```

If the SSH key to access your server isn't added to your [SSH agent](https://www.ssh.com/academy/ssh/agent), specify it
with the `-i` flag:

```shell
uc machine init root@<your-server-ip> -i ~/.ssh/id_xxx
```

This command will:

- Install the latest stable Docker version on your server if it's not already installed
- Install the Uncloud daemon on your server
- Create a Docker network for Uncloud-managed containers
- Deploy [Caddy](https://caddyserver.com/) as your reverse proxy listening on host ports 80 and 443
- Reserve a free `xxxxxx.uncld.dev` subdomain via the Uncloud managed DNS service and point it to your server's IP

All in about a minute!

<details>
<summary>💡 Expand to see example output</summary>

```
$ uc machine init root@157.180.72.195
⏳ Running Uncloud install script...
⏳ Installing Docker...
# Executing docker install script, commit: 53a22f61c0628e58e1d6680b49e82993d304b449
+ sh -c apt-get -qq update >/dev/null
+ sh -c DEBIAN_FRONTEND=noninteractive apt-get -y -qq install ca-certificates curl >/dev/null
+ sh -c install -m 0755 -d /etc/apt/keyrings
+ sh -c curl -fsSL "https://download.docker.com/linux/ubuntu/gpg" -o /etc/apt/keyrings/docker.asc
+ sh -c chmod a+r /etc/apt/keyrings/docker.asc
+ sh -c echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu noble stable" > /etc/apt/sources.list.d/docker.list
+ sh -c apt-get -qq update >/dev/null
+ sh -c DEBIAN_FRONTEND=noninteractive apt-get -y -qq install docker-ce docker-ce-cli containerd.io docker-compose-plugin docker-ce-rootless-extras docker-buildx-plugin >/dev/null

Running kernel seems to be up-to-date.

No services need to be restarted.

No containers need to be restarted.

No user sessions are running outdated binaries.

No VM guests are running outdated hypervisor (qemu) binaries on this host.
+ sh -c docker version
Client: Docker Engine - Community
 Version:           28.2.1
 API version:       1.50
 Go version:        go1.24.3
 Git commit:        879ac3f
 Built:             Wed May 28 19:25:01 2025
 OS/Arch:           linux/amd64
 Context:           default

Server: Docker Engine - Community
 Engine:
  Version:          28.2.1
  API version:      1.50 (minimum version 1.24)
  Go version:       go1.24.3
  Git commit:       0e2cc22
  Built:            Wed May 28 19:25:01 2025
  OS/Arch:          linux/amd64
  Experimental:     false
 containerd:
  Version:          1.7.27
  GitCommit:        05044ec0a9a75232cad458027ca83437aae3f4da
 runc:
  Version:          1.2.5
  GitCommit:        v1.2.5-0-g59923ef
 docker-init:
  Version:          0.19.0
  GitCommit:        de40ad0

================================================================================

To run Docker as a non-privileged user, consider setting up the
Docker daemon in rootless mode for your user:

    dockerd-rootless-setuptool.sh install

Visit https://docs.docker.com/go/rootless/ to learn about rootless mode.


To run the Docker daemon as a fully privileged service, but granting non-root
users access, refer to https://docs.docker.com/go/daemon-access/

WARNING: Access to the remote API on a privileged Docker daemon is equivalent
         to root access on the host. Refer to the 'Docker daemon attack surface'
         documentation for details: https://docs.docker.com/go/attack-surface/

================================================================================

✓ Docker installed successfully.
✓ Linux user and group 'uncloud' created.
⏳ Installing Uncloud binaries...
⏳ Downloading uncloudd binary: https://github.com/psviderski/uncloud/releases/latest/download/uncloudd_linux_amd64.tar.gz
✓ uncloudd binary installed: /usr/local/bin/uncloudd
⏳ Downloading uninstall script: https://raw.githubusercontent.com/psviderski/uncloud/refs/heads/main/scripts/uninstall.sh
✓ uncloud-uninstall script installed: /usr/local/bin/uncloud-uninstall
✓ Systemd unit file created: /etc/systemd/system/uncloud.socket
✓ Systemd unit file created: /etc/systemd/system/uncloud.service
Created symlink /etc/systemd/system/sockets.target.wants/uncloud.socket → /etc/systemd/system/uncloud.socket.
Created symlink /etc/systemd/system/multi-user.target.wants/uncloud.service → /etc/systemd/system/uncloud.service.
⏳ Starting Uncloud machine daemon (uncloud.service)...
✓ Uncloud machine daemon started.
✓ Uncloud installed on the machine successfully! 🎉
Cluster initialised with machine 'machine-dc3c' and saved as context 'default' in your local config (/Users/spy/.config/uncloud/config.yaml)
Current cluster context is now 'default'.
Waiting for the machine to be ready...

Reserved cluster domain: sh8hsb.uncld.dev
[+] Deploying service caddy 2/2
 ✔ Container caddy-d7uk on machine-dc3c     Running                                           11.1s
   ✔ Image caddy:2.11.4 on machine-dc3c     Pulled                                             3.7s

Updating cluster domain records in Uncloud DNS to point to machines running caddy service...
[+] Verifying internet access to caddy service 1/1
 ✔ Machine machine-dc3c (157.180.72.195)  Reachable                                            0.7s

DNS records updated to use only the internet-reachable machines running caddy service:
  *.sh8hsb.uncld.dev  A → 157.180.72.195
```

</details>

## Deploy Excalidraw

Now that your machine is set up, let's deploy `excalidraw` service from the
[official Docker image](https://hub.docker.com/r/excalidraw/excalidraw). The service will publish the container port 80
as HTTPS endpoint on the previously reserved domain via Caddy.

```shell
uc run --name excalidraw --publish 80/https excalidraw/excalidraw
```

You'll see the progress of the deployment and the public URL where you can access the service:

```
[+] Running service excalidraw (replicated mode) 2/2
 ✔ Container excalidraw-azpc on machine-dc3c  Healthy                      37.1s
   ✔ Image excalidraw/excalidraw on machine-dc3c  Pulled                    4.7s

excalidraw endpoints:
 • https://excalidraw.sh8hsb.uncld.dev → :80
```

## Verify your deployment

After the service is deployed, use the `uc inspect` command to check its status and details:

```shell
uc inspect excalidraw
```

```
Service ID: 4d2de1600b6ada221a03896cd388836c
Name:       excalidraw
Mode:       replicated

CONTAINER ID   IMAGE                          CREATED              STATUS                        IP ADDRESS   MACHINE
fde7ac7f11ad   excalidraw/excalidraw:latest   About a minute ago   Up About a minute (healthy)   10.210.0.3   machine-dc3c
```

In this example, the service has one container running on the machine `machine-dc3c` (our server). The container is up
and healthy.

You can also list all deployed services and their public endpoints using the `uc ls` command:

```shell
uc ls
```

```
NAME         MODE         REPLICAS   IMAGE                          ENDPOINTS
caddy        global       1          caddy:2.11.4
excalidraw   replicated   1          excalidraw/excalidraw:latest   https://excalidraw.sh8hsb.uncld.dev → :80
```

You can see `caddy` service listed here. That's your reverse proxy, running as a regular Uncloud service.

## It's live! Start drawing! ✨

Open your browser and navigate to the URL shown in the endpoints. It may take a moment for Caddy to obtain a TLS
certificate from Let's Encrypt. If it doesn't load immediately, wait a few seconds and try again.

![Excalidraw running on Uncloud](https://media.uncloud.run/docs/excalidraw-browser.webp)

You now have:

- Your **own Excalidraw instance** running on your server
- A **public URL** with **automatic HTTPS** you can share with your team and friends
- **Full control over your data** — no analytics or tracking

## View service logs

Want to see what's happening inside your service? Use the `uc logs` command to view logs from the service container:

```shell
uc logs excalidraw
```

```
Jul 14 10:53:05.910 machine-dc3c excalidraw/fde7a ::1 - - [14/Jul/2026:00:53:05 +0000] "GET / HTTP/1.1" 200 6843 "-" "Wget" "-"
Jul 14 10:53:31.456 machine-dc3c excalidraw/fde7a 10.210.0.2 - - [14/Jul/2026:00:53:31 +0000] "GET /sw.js HTTP/1.1" 200 0 "-" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:150.0) Gecko/20100101 Firefox/150.0"
```

Add the `-f` flag to stream new logs in real-time. Press `Ctrl+C` to stop:

```shell
uc logs excalidraw -f
```

## Convert to Docker Compose format

Uncloud supports the [Compose file format](https://docs.docker.com/reference/compose-file/) for defining services. This
allows you to version control your deployments, share configurations with your team, and deploy complex multi-service
applications with a single command.

Let's create a `compose.yaml` file in your current directory for the `excalidraw` service we just deployed.

```yaml title="compose.yaml"
services:
  excalidraw:
    image: excalidraw/excalidraw
    x-ports:
      - 80/https
```

:::info note

The [`x-ports`](../8-compose-file-reference/2-extensions.md#x-ports) key is an Uncloud-specific extension to the Compose
file format. It allows you to specify ports that should be published as HTTP(S) endpoints. Uncloud automatically
configures the reverse proxy (Caddy) to route traffic to these ports.

:::

Now deploy it:

```shell
uc deploy
```

```
Services are up to date.
```

Since `excalidraw` service is already running with the same configuration, Uncloud recognises there's nothing to change.
We've successfully converted our deployment created with `uc run` to a Compose file.

## Use your own domain

Want to use your own domain, for example, `excalidraw.example.com` instead of `excalidraw.sh8hsb.uncld.dev`?

Add a CNAME record `excalidraw.example.com` in your DNS provider (Cloudflare, Namecheap, etc.) pointing to
`excalidraw.sh8hsb.uncld.dev`. Alternatively, you can add an A record pointing to your server's IP.

:::info note

These instructions set up your own domain **in addition to** the Uncloud managed DNS name
`excalidraw.sh8hsb.uncld.dev`.

If you want to avoid the managed service altogether, add `--no-dns` to your `uc machine init` command, and point an A
DNS record to your servers' IPs.

:::

Then update the published port `80/https` in `compose.yaml` to use your domain:

```yaml title="compose.yaml"
services:
  excalidraw:
    image: excalidraw/excalidraw
    x-ports:
      - excalidraw.example.com:80/https
```

Finally, deploy the changes:

```shell
uc deploy
```

```
Deployment plan

context: default

~ update service excalidraw
  │   image: excalidraw/excalidraw:latest
  │
  ╰── +/- replace container excalidraw/fde7ac7f11ad on machine-dc3c

──────────────────────────────────────────
1 replace (start-first) · across 1 machine

Proceed with deployment to default? [y/N] y

[+] Deploying to default 2/2
 ✔ Container excalidraw-0z12 on machine-dc3c          Healthy              30.6s
 ✔ Container excalidraw/fde7ac7f11ad on machine-dc3c  Removed               0.4s
```

Uncloud prints a deployment plan and asks for confirmation before making any changes. The plan says it will replace the
running container with a new one using the
[`start-first` order](../4-guides/1-deployments/4-rolling-deployments.md#update-order). This means Uncloud starts the
new container with the updated configuration, waits for it to become healthy, and only then removes the old one. Your
service stays available throughout the update. That's a **zero-downtime deployment**.

Give it a moment for Caddy to obtain a TLS certificate, then visit https://excalidraw.example.com (use your own domain).

## Clean up

When you're done experimenting, you can remove the `excalidraw` service or uninstall Uncloud completely.

### Remove the service

Remove the `excalidraw` service while keeping your Uncloud machine running for future deployments:

```shell
uc rm excalidraw
```

### Uninstall Uncloud

If you want to completely uninstall Uncloud from your server and clean up everything it created, SSH into your server
and run:

```shell
sudo uncloud-uninstall
```

This command will:

- Remove all Uncloud-managed containers (including Caddy)
- Remove the Uncloud-managed Docker and WireGuard networks
- Uninstall the Uncloud daemon from the server

<details>
<summary>💡 Expand to see example output</summary>

```
⚠️ This script will uninstall Uncloud and remove ALL Uncloud managed containers on this machine.
The following actions will be performed:
- Remove Uncloud systemd services
- Remove Uncloud binaries and data
- Remove Uncloud user and group
- Remove all Docker containers managed by Uncloud
- Remove Uncloud Docker network
- Remove Uncloud WireGuard interface
Do you want to proceed with uninstallation? [y/N] y
⏳ Stopping systemd services...
Removed /etc/systemd/system/multi-user.target.wants/uncloud.service.
Removed /etc/systemd/system/sockets.target.wants/uncloud.socket.
✓ Systemd services stopped.
⏳ Removing systemd service files...
removed '/etc/systemd/system/uncloud.service'
removed '/etc/systemd/system/uncloud.socket'
✓ Systemd service files removed.
⏳ Removing binaries...
removed '/usr/local/bin/uncloudd'
✓ Binaries removed.
⏳ Removing uncloudd-managed corrosion Docker container...
uncloud-corrosion
✓ uncloud-corrosion container removed.
⏳ Removing data and run directories...
removed '/var/lib/uncloud/machine.db-shm'
removed '/var/lib/uncloud/machine.db'
removed '/var/lib/uncloud/corrosion/store.db'
removed '/var/lib/uncloud/corrosion/subscriptions/754e24df40f8476389cf6dbfa7b542c8/sub.sqlite'
removed directory '/var/lib/uncloud/corrosion/subscriptions/754e24df40f8476389cf6dbfa7b542c8'
removed '/var/lib/uncloud/corrosion/subscriptions/125e6ada8eec4f3cad192e1890db55c2/sub.sqlite'
removed directory '/var/lib/uncloud/corrosion/subscriptions/125e6ada8eec4f3cad192e1890db55c2'
removed directory '/var/lib/uncloud/corrosion/subscriptions'
removed '/var/lib/uncloud/corrosion/config.toml'
removed '/var/lib/uncloud/corrosion/schema.sql'
removed directory '/var/lib/uncloud/corrosion'
removed '/var/lib/uncloud/machine.json'
removed '/var/lib/uncloud/caddy/caddy.json'
removed '/var/lib/uncloud/caddy/caddy/autosave.json'
removed '/var/lib/uncloud/caddy/caddy/last_clean.json'
removed directory '/var/lib/uncloud/caddy/caddy/locks'
removed '/var/lib/uncloud/caddy/caddy/instance.uuid'
removed '/var/lib/uncloud/caddy/caddy/acme/acme-staging-v02.api.letsencrypt.org-directory/users/default/default.json'
removed '/var/lib/uncloud/caddy/caddy/acme/acme-staging-v02.api.letsencrypt.org-directory/users/default/default.key'
removed directory '/var/lib/uncloud/caddy/caddy/acme/acme-staging-v02.api.letsencrypt.org-directory/users/default'
removed directory '/var/lib/uncloud/caddy/caddy/acme/acme-staging-v02.api.letsencrypt.org-directory/users'
removed directory '/var/lib/uncloud/caddy/caddy/acme/acme-staging-v02.api.letsencrypt.org-directory'
removed '/var/lib/uncloud/caddy/caddy/acme/acme-v02.api.letsencrypt.org-directory/users/default/default.json'
removed '/var/lib/uncloud/caddy/caddy/acme/acme-v02.api.letsencrypt.org-directory/users/default/default.key'
removed directory '/var/lib/uncloud/caddy/caddy/acme/acme-v02.api.letsencrypt.org-directory/users/default'
removed directory '/var/lib/uncloud/caddy/caddy/acme/acme-v02.api.letsencrypt.org-directory/users'
removed directory '/var/lib/uncloud/caddy/caddy/acme/acme-v02.api.letsencrypt.org-directory/challenge_tokens'
removed directory '/var/lib/uncloud/caddy/caddy/acme/acme-v02.api.letsencrypt.org-directory'
removed directory '/var/lib/uncloud/caddy/caddy/acme'
removed '/var/lib/uncloud/caddy/caddy/certificates/acme-v02.api.letsencrypt.org-directory/excalidraw.sh8hsb.uncld.dev/excalidraw.sh8hsb.uncld.dev.key'
removed '/var/lib/uncloud/caddy/caddy/certificates/acme-v02.api.letsencrypt.org-directory/excalidraw.sh8hsb.uncld.dev/excalidraw.sh8hsb.uncld.dev.crt'
removed '/var/lib/uncloud/caddy/caddy/certificates/acme-v02.api.letsencrypt.org-directory/excalidraw.sh8hsb.uncld.dev/excalidraw.sh8hsb.uncld.dev.json'
removed directory '/var/lib/uncloud/caddy/caddy/certificates/acme-v02.api.letsencrypt.org-directory/excalidraw.sh8hsb.uncld.dev'
removed directory '/var/lib/uncloud/caddy/caddy/certificates/acme-v02.api.letsencrypt.org-directory'
removed directory '/var/lib/uncloud/caddy/caddy/certificates'
removed directory '/var/lib/uncloud/caddy/caddy'
removed '/var/lib/uncloud/caddy/Caddyfile'
removed directory '/var/lib/uncloud/caddy'
removed '/var/lib/uncloud/machine.db-wal'
removed directory '/var/lib/uncloud'
removed '/run/uncloud/caddy/admin.sock'
removed directory '/run/uncloud/caddy'
removed '/run/uncloud/corrosion/admin.sock'
removed directory '/run/uncloud/corrosion'
removed directory '/run/uncloud'
✓ Data and run directories removed.
⏳ Removing Linux user and group...
✓ Linux user 'uncloud' removed.
Linux group 'uncloud' does not exist or was already removed.
⏳ Looking for Docker containers and network created by Uncloud...
Found 1 Uncloud managed containers.
⏳ Stopping Uncloud managed containers...
b2eb9968e468
⏳ Removing Uncloud managed containers...
b2eb9968e468
✓ Uncloud managed containers stopped and removed.
⏳ Removing Docker network uncloud...
uncloud
✓ Docker network uncloud removed.
⏳ Removing WireGuard interface uncloud...
✓ WireGuard interface uncloud removed.
⏳ Removing uninstall script...
removed '/usr/local/bin/uncloud-uninstall'
✓ Uninstall script removed.

✅ Uncloud has been uninstalled successfully!
Note: Docker installation was preserved. If you want to completely remove Docker as well, follow https://docs.docker.com/engine/install/ubuntu/#uninstall-docker-engine
```

</details>

## Further reading

- **[Add more machines](../9-cli-reference/uc_machine_add.md)**: Scale horizontally by creating a cluster of machines
- **[Ingress & HTTP](../3-concepts/2-ingress/1-overview.md)**: Learn how Uncloud handles incoming traffic and how to
  expose your services to the internet
- **[CLI reference](../9-cli-reference/uc.md)**: Explore all available commands and options
