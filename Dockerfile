ARG ALPINE_VERSION=3.23.3

FROM --platform=${BUILDPLATFORM} golang:1.26.1-alpine AS uncloudd

ARG TARGETOS
ARG TARGETARCH

ENV CGO_ENABLED=0

WORKDIR /build
# Download and cache dependencies and only redownload them in subsequent builds if they change.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o uncloudd ./cmd/uncloudd


FROM alpine:${ALPINE_VERSION} AS corrosion-download
ARG TARGETARCH
ARG CORROSION_VERSION

RUN CORROSION_ARCH=$(case "${TARGETARCH}" in \
      "amd64") echo "x86_64" ;; \
      "arm64") echo "aarch64" ;; \
      *) echo "Architecture '${TARGETARCH}' not supported" >&2 && exit 1 ;; \
    esac) \
    && wget -q -O /tmp/corrosion.tar.gz \
      "https://github.com/psviderski/corrosion/releases/download/v${CORROSION_VERSION}/corrosion-${CORROSION_ARCH}-unknown-linux-gnu.tar.gz" \
    && tar -xzf /tmp/corrosion.tar.gz -C /tmp \
    && install /tmp/corrosion /usr/bin/corrosion \
    && rm /tmp/corrosion.tar.gz /tmp/corrosion

# Beware that more modern images like chainguard/wolfi-base build glibc with flags that require newer CPU features,
# e.g. x86-64-v2. So such image may fail with "Fatal glibc error: CPU does not support x86-64-v2" on older CPUs.
FROM gcr.io/distroless/cc-debian13:latest AS corrosion
COPY --from=corrosion-download /usr/bin/corrosion /usr/bin/corrosion
CMD ["corrosion", "agent"]


FROM --platform=${BUILDPLATFORM} alpine:${ALPINE_VERSION} AS corrosion-image-tarball
ARG CORROSION_VERSION
ARG CORROSION_IMAGE="ghcr.io/unlabs-dev/corrosion:${CORROSION_VERSION}"
ARG TARGETOS
ARG TARGETARCH

RUN apk --no-cache add crane
RUN crane pull --platform ${TARGETOS}/${TARGETARCH} "${CORROSION_IMAGE}" /corrosion.tar

# Uncloud-in-Docker (ucind) image for running Uncloud test clusters using Docker.
FROM --platform=${BUILDPLATFORM} docker:29.4.0-dind AS ucind
ARG CORROSION_VERSION

# Create system group and user 'uncloud'.
RUN addgroup -S uncloud && adduser -SHD -h /nonexistent -G uncloud -g "" uncloud
RUN apk --no-cache add \
    socat \
    wireguard-tools

COPY --from=corrosion-image-tarball /corrosion.tar /images/corrosion.tar
COPY scripts/docker/entrypoint.sh /usr/local/bin/
COPY --from=uncloudd /build/uncloudd /usr/local/bin/

ENTRYPOINT ["entrypoint.sh"]
CMD ["uncloudd"]
