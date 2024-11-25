FROM golang:1.23.2-alpine AS uncloudd

ARG TARGETOS
ARG TARGETARCH

COPY . .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o uncloudd cmd/uncloudd/main.go


FROM docker:27.3.1-dind
RUN apk --no-cache add \
    wireguard-tools
COPY --from=uncloudd /go/uncloudd /usr/local/bin/uncloudd
CMD ["uncloudd"]
