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
RUN wget -q -O /tmp/corrosion.tar.gz \
      https://github.com/psviderski/corrosion/releases/latest/download/corrosion-aarch64-unknown-linux-gnu.tar.gz \
    && tar -xzf /tmp/corrosion.tar.gz -C /tmp \
    && install /tmp/corrosion /usr/local/bin/corrosion \
    && rm /tmp/corrosion.tar.gz /tmp/corrosion

FROM chainguard/wolfi-base:latest AS corrosion
COPY --from=corrosion-download /usr/local/bin/corrosion /usr/local/bin/corrosion
CMD ["corrosion", "agent"]


FROM alpine:${ALPINE_VERSION} AS corrosion-image-tarball
ARG CORROSION_IMAGE="ghcr.io/psviderski/corrosion:latest"

RUN apk --no-cache add crane
RUN crane pull "${CORROSION_IMAGE}" /corrosion.tar


FROM docker:27.3.1-dind AS machine
RUN apk --no-cache add \
    socat \
    wireguard-tools
# Create system group and user 'uncloud'.
RUN addgroup -S uncloud && adduser -SHD -h /nonexistent -G uncloud -g "" uncloud

COPY --from=corrosion-image-tarball /corrosion.tar /images/corrosion.tar
COPY scripts/docker/dind scripts/docker/entrypoint.sh /usr/local/bin/
COPY --from=uncloudd /build/uncloudd /usr/local/bin/
# TODO: socat to forward uncloud.sock unix socket?

ENTRYPOINT ["entrypoint.sh"]
CMD ["uncloudd"]
