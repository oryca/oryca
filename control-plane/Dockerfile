# Built from the repository root, not from this directory: the whole repo is one
# Go module, so go.mod/go.sum live at the root.
#   docker build -f control-plane/Dockerfile .
FROM --platform=$BUILDPLATFORM golang:1.25.7-bookworm AS builder

ENV GO111MODULE=on
ENV CGO_ENABLED=0

# Compile on the machine running the build and emit for the target: with cgo off
# Go cross-compiles natively, which is far quicker than emulating the target.
ARG TARGETOS
ARG TARGETARCH


# Dependencies first so that editing source does not re-download the module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/oryca-control-plane/ cmd/oryca-control-plane/
COPY control-plane/ control-plane/
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/oryca-control-plane -ldflags "-w -s" ./cmd/oryca-control-plane

# Alpine for the runtime: the binary is static (CGO is off), so all the image has
# to carry is the CA bundle, zoneinfo, curl for the health check, and openssl.
#
# openssl generates the JWT signing key on first start (see docker-entrypoint.sh).
# Runs unprivileged: /data holds that key and is mounted as a volume, so it is
# created here with the right owner to make the fresh volume writable.
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata curl openssl \
  && adduser -S -u 10001 -G nogroup oryca \
  && mkdir -p /data \
  && chown oryca:nogroup /data

COPY --from=builder /out/oryca-control-plane /usr/local/bin/
COPY --chmod=755 control-plane/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

USER oryca
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
