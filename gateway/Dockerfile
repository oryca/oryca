# Built from the repository root, not from this directory: the whole repo is one
# Go module, so go.mod/go.sum live at the root.
#   docker build -f gateway/Dockerfile .
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

COPY cmd/oryca-gateway/ cmd/oryca-gateway/
COPY gateway/ gateway/
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/oryca-gateway -ldflags "-w -s" ./cmd/oryca-gateway

# Alpine for the runtime: the binary is static (CGO is off), so all the image has
# to carry is the CA bundle, zoneinfo, and curl for the container health check.
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata curl \
  && adduser -S -u 10001 -G nogroup oryca

COPY --from=builder /out/oryca-gateway /usr/local/bin/

USER oryca

CMD ["oryca-gateway"]
