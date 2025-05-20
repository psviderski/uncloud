ARG ALPINE_VERSION=3.20.3

FROM golang:1.23.2-alpine AS uncloudd

ARG TARGETOS
ARG TARGETARCH

ENV CGO_ENABLED=0

WORKDIR /build
# Download and cache dependencies and only redownload them in subsequent builds if they change.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o uncloudd cmd/uncloudd/main.go


FROM alpine:${ALPINE_VERSION} AS corrosion-download
ARG TARGETARCH

RUN CORROSION_ARCH=$(case "${TARGETARCH}" in \
      "amd64") echo "x86_64" ;; \
      "arm64") echo "aarch64" ;; \
      *) echo "Architecture '${TARGETARCH}' not supported" >&2 && exit 1 ;; \
    esac) \
    && wget -q -O /tmp/corrosion.tar.gz \
      "https://github.com/psviderski/corrosion/releases/latest/download/corrosion-${CORROSION_ARCH}-unknown-linux-gnu.tar.gz" \
    && tar -xzf /tmp/corrosion.tar.gz -C /tmp \
    && install /tmp/corrosion /usr/local/bin/corrosion \
    && rm /tmp/corrosion.tar.gz /tmp/corrosion

# Beware that more modern images like chainguard/wolfi-base build glibc with flags that require newer CPU features,
# e.g. x86-64-v2. So such image may fail with "Fatal glibc error: CPU does not support x86-64-v2" on older CPUs.
FROM gcr.io/distroless/cc-debian12:latest AS corrosion
COPY --from=corrosion-download /usr/local/bin/corrosion /usr/local/bin/corrosion
CMD ["corrosion", "agent"]


FROM alpine:${ALPINE_VERSION} AS corrosion-image-tarball
ARG CORROSION_IMAGE="ghcr.io/psviderski/corrosion:latest"

RUN apk --no-cache add crane
RUN crane pull "${CORROSION_IMAGE}" /corrosion.tar


# Uncloud-in-Docker (ucind) image for running Uncloud test clusters using Docker.
FROM docker:27.3.1-dind AS ucind
# Create system group and user 'uncloud'.
RUN addgroup -S uncloud && adduser -SHD -h /nonexistent -G uncloud -g "" uncloud
RUN apk --no-cache add \
    socat \
    wireguard-tools

COPY --from=corrosion-image-tarball /corrosion.tar /images/corrosion.tar
COPY scripts/docker/dind scripts/docker/entrypoint.sh /usr/local/bin/
COPY --from=uncloudd /build/uncloudd /usr/local/bin/

ENTRYPOINT ["entrypoint.sh"]
CMD ["uncloudd"]
