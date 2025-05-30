# Proposal: Application Builds from Source

This proposal aims to support Compose-style builds and deployments for services from source, in addition to the existing functionality for deploying public container images. The proposed behavior should closely align with [Docker Compose](https://docs.docker.com/compose/) and the (platform-agnostic) [Compose spec](https://github.com/compose-spec/compose-spec) in terms of building and deploying local applications from source. This will ensure a smooth transition for current Compose users.

### Example

The `uc deploy` command should be able to build and deploy a Compose file like the following:

```yaml
services:
  nginx:
    build:
      context: nginx/
      dockerfile: nginx/Dockerfile
      args:
        GIT_COMMIT: abc1234
```

In this configuration, the nginx service does not specify an image to use (via the image: directive). Instead, it defines a local build context directory (nginx/) and a Dockerfile (nginx/Dockerfile) to build the image. Additionally, it includes a build argument (GIT_COMMIT) to use during the build process.

The full Compose build specification is available [here](https://github.com/compose-spec/compose-spec/blob/main/build.md). While it is not necessary to implement the entire specification initially, supporting as many attributes as possible would enhance compatibility with Compose and provide a better user experience.

To implement this feature, two major components need to be addressed, primarily concerning the lifecycle of the intermediate container images that are built and deployed:

1. How and where the intermediate images are built
2. Where the intermediate images are stored and how they are distributed

Both problems are covered in the sections below.

### Out of scope

1. Supporting alternative image build systems (buildpack, nixpack, etc.)
1. Supporting alternative build engines (kaniko, buildah, etc.)

## Implementation options: Building images

Building images from application source typically involves a context folder and a Dockerfile. The key question is: where should the build process take place?

### Option A: Off-cluster builds (local or remote)

Idea: Perform the build locally (on the same machine where the Compose file and application source are located) or remotely (on a different Docker-enabled machine, e.g., via a [SSH-tunneled context](https://blog.mikesir87.io/2019/08/using-ssh-connections-in-docker-contexts/)).

**Pros:**

- Supported natively by Docker; you only need to configure `DOCKER_HOST` or `DOCKER_CONTEXT` accordingly.
- Cluster nodes and their workloads remain unaffected.
- (Local builds) Faster builds since the build context does not need to be transferred over the network.

**Cons:**

- The resulting image must be uploaded to the cluster, adding extra steps.
- (Local builds) Requires a local Docker engine (or equivalent), introducing a dependency not previously needed.

### Option B: In-cluster builds

Idea: Use a remote Docker engine on one of the cluster nodes to build the image. The build context is transferred over the network, and the image is built on one of the cluster nodes.

**Pros:**

- Keeps the client lightweight, avoiding the need for a local Docker daemon.
- The built image is already stored on a cluster node, simplifying subsequent deployment steps.

**Cons:**

- Slower build times due to the network transfer of the build context.
- Resource-intensive builds could impact the performance of the cluster node.

## Implementation options: Image storage and distribution

After images are built from source, they need to be stored and made available on the node where the container will run.

### Option 1: Internal registry

Idea: Deploy an internal image registry (e.g., the official [distribution registry](https://hub.docker.com/_/registry)) to store build artifacts. When launching a container, the image is referenced using a URL pointing to the registry.

**Pros:**

- Provides a dedicated component for image storage and distribution.
- Can act as a local cache for public images, improving performance.

**Cons:**

- Adds an additional component to the cluster, increasing overhead.
- If the registry or its hosting node goes down, no new images can be built or retrieved.
- Image storage is essentially duplicated (if using [filesystem storage driver](https://distribution.github.io/distribution/storage-drivers/filesystem/)) as some images will be stored in both the registry and Docker image store on machines.
- The registry needs to be securely exposed to the internet (e.g. using basic auth -- not supported yet) or locally through port forwarding or WireGuard connection (not supported yet).
- Images in registry need to be deleted and garbage collected, e.g. using [regclient](https://github.com/regclient/regclient) or automated policies, so that the occupied storage doesn't grow infinitely.

### Option 2: Peer-to-peer image storage

Idea: Store the built image on one or multiple cluster nodes. When another node requires the image, its location is determined via distributed state (Corrosion), and the image is transferred over the cluster network. Ultimately, each node may expose the Registry API backed by local image store so that regular push/pulls could be used to efficiently transfer only required layers.

**Pros:**

- No need for additional components.
- Simplifies the architecture by leveraging existing cluster nodes.

**Cons:**

- Requires a custom implementation for image discovery and sharing.
- Each cluster node must allocate sufficient disk space for image storage.
- Each push/pull will require to transfer the entire image unless a more complex logic for transferring only new/missing layers or Registry API is implemented.

### Option 3: External registry

Idea: Upload built images to an external registry (such as Docker Hub, GHCR registry, AWS ECR).

**Pros:**

- Avoids runtime overhead or new components inside the cluster.
- External storage solutions are often highly reliable and scalable.

**Cons:**

- Likely requires authentication and integration with an external service.
- Introduces dependency on a potentially proprietary external provider.
