# Deploy an app

Deploy a containerised application to your Uncloud cluster using either source code or pre-built images.

This guide covers both scenarios:

- **[Deploy from source code](#deploy-from-source-code)**: Build Docker images from your application code and use them
  to deploy service containers
- **[Deploy pre-built images](#deploy-pre-built-images)**: Deploy service containers using existing images from a
  registry or your local machine

Uncloud uses [Compose Specification](https://compose-spec.io/) for defining the deployment configuration of your app's
services. It implements the most common Compose features with some Uncloud-specific extensions. See
[Compose support matrix](../../8-compose-file-reference/1-support-matrix.md) for details.

## Prerequisites

- `uc` CLI [installed](../../2-getting-started/1-install-cli.md) on your local machine
- An Uncloud cluster with at least one machine (see [Quick start](../../2-getting-started/2-deploy-demo-app.md))
- Basic knowledge of [Compose Specification](https://compose-spec.io/)

## Deploy from source code

Use this scenario when you want to build and deploy your application from
a [Dockerfile](https://docs.docker.com/reference/dockerfile/) and source code available on your local machine.

### 1. Create a Compose file

Create a `compose.yaml` file in your application directory with a
[`build`](https://github.com/compose-spec/compose-spec/blob/main/spec.md#build) section for each service you want to
build. Here is a minimal example of a Compose file that builds and deploys a web app that consists of a single service
called `web`:

```yaml title="compose.yaml"
services:
  web:
    # Build an image from the Dockerfile in the current directory
    build: .
    # Publish the container port 8000 as https://app.example.com
    x-ports:
      - app.example.com:8000/https
```

If you don't have a Dockerfile for building an image from your source code, create one in the same directory.

### 2. Build and deploy your app

To build and deploy services defined in your Compose file, navigate to the directory with `compose.yaml` and run:

```shell
uc deploy
```

This command looks for a Compose file in your current working directory and:

1. **Builds images** for services with a `build` section using your local Docker and tags them with the current Git
   version
2. **Pushes built images** directly **to cluster machines** using
   [unregistry](https://github.com/psviderski/unregistry), transferring only the missing layers
3. **Plans the deployment** and shows you what will change, asking for confirmation
4. **Creates any missing volumes** on target machines
5. **Deploys service containers** using the built images and latest configuration changes with zero-downtime rolling
   updates

See the [`uc deploy` ](../../9-cli-reference/uc_deploy.md) reference for all available options.

Watch `uc deploy` building and deploying a demo app (18 seconds):

<video controls width="100%">
  <source src="https://media.uncloud.run/docs/uc-deploy-demo.mp4"/>
</video>

### Customise the build configuration

If you need more control over the build configuration, you can specify additional attributes in the `build` section of
your Compose file. For example, to specify a custom Dockerfile location, set build-time arguments, or build
multi-platform images.

```yaml
services:
  web:
    build:
      # Relative path to the directory with your Dockerfile
      context: ./backend
      # Set build-time variables defined as ARG in your Dockerfile
      args:
        ALPINE_VERSION: 3.22
        BUILD_ENV: prod
      # Build a multi-platform image
      platforms:
        - linux/amd64
        - linux/arm64
```

You can pass additional build arguments or override existing ones using the `--build-arg` flag with `uc deploy`:

```shell
uc deploy --build-arg BUILD_ENV=dev
```

You can also use advanced features like build caches or SSH access.
See [Compose Build Specification](https://github.com/compose-spec/compose-spec/blob/main/build.md) for all supported
attributes. Note that build [secrets](https://github.com/compose-spec/compose-spec/blob/main/build.md#secrets) are not
supported though.

:::info note

To build multi-platform images, you need to configure your local Docker to use the
[containerd image store](https://docs.docker.com/desktop/features/containerd/).

:::

### Customise image tags

If you don't specify the `image` attribute, `uc deploy` tags built images with a Git-based version like

```yaml
# <project>/<service>:<git datetime>.<short git sha>
myapp/web:2025-10-30-223604.84d33bb
```

It uses your local date/time if the working directory is not a Git repository.

You can customise the image name and tag format using the `image` attribute. It can be a static name or a dynamic
template using [environment variables](https://github.com/compose-spec/compose-spec/blob/main/spec.md#interpolation) and
the [Go template](https://pkg.go.dev/text/template) syntax.

```yaml
services:
  web:
    build: .
    # Custom image name and tag format
    image: myapp:{{gitdate "20060102"}}.{{gitsha 7}}.${GITHUB_RUN_ID:-local}{{if .Git.IsDirty}}.dirty{{end}}
```

This generates tags like:

- `myapp:20251030.84d33bb.local.dirty` when building locally with uncommitted changes
- `myapp:20251030.84d33bb.1234` when building in CI with `GITHUB_RUN_ID=1234` and a clean repo

`uc deploy` renders the image templates when it loads the Compose file and then uses the resulting names for the build
and deploy stages.

See the [Image tag template](../../8-compose-file-reference/2-image-tag-template.md) reference for all available
template variables and functions.

### Separate build and deploy steps

You might want to build and deploy services as separate commands. For example, to run them as separate steps in your
CI/CD pipeline or have more control over the build and deploy process. Here are the commands that are equivalent to
`uc deploy`:

```shell
# Build images and push to cluster machines
uc build --push

# Deploy services using the built images
uc deploy --no-build
```

### Deploy configuration changes only

You can use the `--no-build` flag with `uc deploy` to deploy only the configuration changes in your Compose file if your
source code and images haven't changed. However, the image tag may still change if you're using
[dynamic tags](#customise-image-tags) based on the Git state (default). In that case, the deploy will likely fail
because the new tag won't be found on cluster machines.

Use less sensitive dynamic tags or specify the built and pushed image tags you want to deploy explicitly to avoid this
issue.

The recommended approach though is to commit all your changes, including the configuration ones, to the repo. Then
rebuild and deploy your services from a clean repo state. This way, the image tags will always reflect the exact source
code and configuration used for the deployment.

## Deploy pre-built images

Use this scenario when you want to deploy your application using pre-built images from a container registry (for
example, Docker Hub or GitHub Container Registry) or your local Docker.

### 1. Create a Compose file

Create a `compose.yaml` file that references an image from a registry using the
[`image`](https://github.com/compose-spec/compose-spec/blob/main/spec.md#image) attribute. It's important that you don't
include a `build` section for services using pre-built images. Here is a minimal example of a Compose file that deploys
an app that consists of a single `nginx` service:

```yaml title="compose.yaml"
services:
  nginx:
    # Use the nginx image from Docker Hub
    image: nginx:latest
    # Publish the container port 80 as https://nginx.example.com
    x-ports:
      - nginx.example.com:80/https
```

### 2. Deploy your app

To deploy services defined in your Compose file, navigate to the directory with `compose.yaml` and run:

```shell
uc deploy
```

This command looks for a Compose file in your current working directory and:

1. **Plans the deployment** and shows you what will change, asking for confirmation
2. **Creates any missing volumes** on target machines
3. **Pulls images** from a registry on cluster machines where services are deployed according to the
   [`pull_policy`](https://github.com/compose-spec/compose-spec/blob/main/spec.md#pull_policy)
4. **Deploys service containers** using the pulled images and latest configuration changes with zero-downtime rolling
   updates

### Control image pulling

By default, `uc deploy` pulls an image from a registry only if it's missing on a target machine. You can change this
behavior using the [`pull_policy`](https://github.com/compose-spec/compose-spec/blob/main/spec.md#pull_policy)
attribute. For example, to always pull the latest version of an image before deploying.

```yaml
services:
  nginx:
    image: nginx:alpine
    # Always pull the latest :alpine tag before deploying
    pull_policy: always
```

Available `pull_policy` values:

- `always`: Always pull the image from the registry before deploying
- `missing` (default): Pull only if the image isn't available on the target machine
- `never`: Never pull, the image must be present on the target machine or the deploy will fail

### Pull from a private registry

If your images are in a private registry, `uc deploy` needs an authentication token to pull them. You can provide it by
either:

- Logging in to the registry using your local Docker (recommended)
- Logging in to the registry using Docker on each cluster machine you want to deploy to

When you're logged in using your local Docker, `uc deploy` automatically passes your local Docker credentials for the
private registry to cluster machines when pulling images. This way, you don't need to log in on each machine manually.

See [`docker login`](https://docs.docker.com/reference/cli/docker/login/) for instructions on how to log in to a private
registry.

### Push local images to cluster machines

You can also deploy pre-built images that exist only in your local Docker and are not available for pulling on cluster
machines. For example, images you built locally outside of the Compose workflow or pulled from a private registry that
is unreachable from your cluster machines. This is useful for deploying to air-gapped or restricted environments.

You can push these local images directly to your cluster machines using the
[`uc image push`](../../9-cli-reference/uc_image_push.md) command:

```shell
# Push to *all* cluster machines
uc image push myapp:latest

# Push to specific machines only
uc image push myapp:latest -m machine1,machine2
```

This command uploads the image from your local Docker to the cluster machines using
[unregistry](https://github.com/psviderski/unregistry) running as part of the Uncloud daemon on each machine. It
efficiently transfers only the image layers that don't already exist on the target machines.

After pushing the image, update your Compose file to reference it:

```yaml
services:
  web:
    # Reference the image you just pushed
    image: myapp:latest
    # Never try to pull from a registry since the image is only available locally
    pull_policy: never
```

Run [`uc images`](../../9-cli-reference/uc_images.md) to verify that the image is available on the target machines.

Then deploy as usual:

```shell
uc deploy
```

:::tip

Set `pull_policy: never` when using local images to prevent `uc deploy` from trying to pull them from a registry.

:::

## Mix source builds and pre-built images

Define multiple services in your Compose file, mixing source builds and pre-built images:

```yaml
services:
  web:
    # Build web service from source
    build: .
    x-ports:
      - app.example.com:8000/https

  db:
    # Pull a pre-built PostgreSQL image from Docker Hub
    image: postgres:18
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD}
```

`uc deploy` builds and pushes images for services with a `build` section and pulls images from a registry for services
with only an `image` attribute.

:::warning

**Service names must be globally unique** across all Compose files deployed to the same cluster.

Unlike Docker Compose or Docker Swarm, Uncloud doesn't automatically prefix service names with project or stack names.
Choose unique names to avoid conflicts with services deployed from other Compose files.

:::

## Use a different Compose file location

If your Compose file has a different name or location, use the `-f/--file` flag to specify its path:

```shell
uc deploy -f path/to/your-compose.yaml
```

You can also specify multiple Compose files to merge configurations:

```shell
uc deploy -f compose.yaml -f compose.prod.yaml
```

See [Use multiple Compose files](https://docs.docker.com/compose/how-tos/multiple-compose-files/) for details on how you
can customise your Compose application for different environments or workflows.

## Verify your deployment

After deploying your app, you can verify that your services are running as expected by listing all deployed services in
the cluster:

```shell
uc ls
```

and inspecting the status of containers for a specific service:

```shell
uc inspect web
```

# See also

- [Deploy to specific machines](2-deploy-specific-machines.md): Deploy services to specific machines in your cluster
- [Deploy a global service](3-deploy-global-services.md): Deploy one service replica on each cluster machine
- [Compose Specification](https://compose-spec.io/): Official specification for the Compose file format
- [Compose support matrix](../../8-compose-file-reference/1-support-matrix.md): Supported Compose features and Uncloud
  extensions
