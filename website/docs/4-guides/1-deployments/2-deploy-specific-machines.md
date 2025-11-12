# Deploy to specific machines

Deploy services to specific machines in your cluster using the
[`x-machines`](../../8-compose-file-reference/1-support-matrix.md#x-machines) extension in your Compose file.

## When to target specific machines

By default, Uncloud randomly chooses available machines to run your services on, evenly spreading multiple replicas of a
service across all machines for high availability. You can restrict which machines can run your service using the
[`x-machines`](../../8-compose-file-reference/1-support-matrix.md#x-machines) extension in your Compose file.

This is useful when you want to:

- Deploy services to machines in a **specific region** or **data center**
- Run services only on machines with **specific hardware** (for example, GPUs or ARM processors)
- Specify where to deploy **stateful services** and create their data volumes
- Keep certain services isolated to **dedicated machines**
- Deploy to a subset of machines for **testing** before rolling out cluster-wide

## Target machines in a Compose file

Set `x-machines` to a list of machine names or a single machine name to restrict which machines a service can run on.

```yaml title="compose.yaml"
services:
  web:
    build: .
    x-ports:
      - app.example.com:8000/https
    # Spread 3 replicas across machine-1 and machine-2 only
    x-machines:
      - machine-1
      - machine-2
    scale: 3

  db:
    image: postgres:18
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - db-data:/var/lib/postgresql
    # Create the db-data volume and run the DB container on machine-db
    x-machines: machine-db

volumes:
  db-data:
```

When you deploy this Compose file with [`uc deploy`](../../9-cli-reference/uc_deploy.md):

- The `web` service will create and spread its 3 replicas only across `machine-1` and `machine-2`
- The `db` service will create its `db-data` volume and run only on `machine-db`

:::tip

Use [`uc machine ls`](../../9-cli-reference/uc_machine_ls.md) to see available machines in your cluster.

:::

## Push images to specific machines only

When [building from source](1-deploy-app.md#deploy-from-source-code), `uc deploy` and `uc build --push` automatically
push built images to **all** cluster machines by default. This ensures images are available wherever services might be
deployed.

If you're using `x-machines` to restrict deployments to specific machines, `uc deploy` and `uc build --push` push images
**only to those** machines. This saves time and bandwidth by uploading images only where they're needed.

You can also manually push your local Docker images to specific machines using the [
`uc image push`](../../9-cli-reference/uc_image_push.md) command:

```shell
# Push the local Docker image 'myapp:latest' to machine-1 and machine-2
uc image push myapp:latest -m machine-1,machine-2
```

See [Push local images to cluster machines](1-deploy-app.md#push-local-images-to-cluster-machines) for more details.

## See also

- [Deploy a global service](3-deploy-global-services.md): Deploy one service replica on each cluster machine
- [Deploy an app](1-deploy-app.md): Deploy from source code or prebuilt images
- [Compose support matrix](../../8-compose-file-reference/1-support-matrix.md): Supported Compose features and Uncloud
  extensions
