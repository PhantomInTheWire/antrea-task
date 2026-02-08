ARG GO_VERSION=1.25

FROM golang:${GO_VERSION} AS builder

WORKDIR /build

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=antrea-capture/go.mod,target=go.mod \
    --mount=type=bind,source=antrea-capture/go.sum,target=go.sum \
    go mod download

COPY antrea-capture/ .

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build/ \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o antrea-capture .

FROM ubuntu:24.04

LABEL maintainer="Karan Lokchandani"
LABEL description="Antrea PacketCapture controller Pretest"

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        bash \
        tcpdump \
        util-linux && \
    rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

COPY --from=builder /build/antrea-capture /usr/local/bin/antrea-capture

USER root

ENTRYPOINT ["antrea-capture"]
