FROM golang:1.23 AS builder

WORKDIR /build
COPY antrea-capture/go.mod antrea-capture/go.sum ./

RUN go mod download
COPY antrea-capture/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o antrea-capture .

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y tcpdump
COPY --from=builder /build/antrea-capture /usr/local/bin/antrea-capture

USER root
ENTRYPOINT ["antrea-capture"]
