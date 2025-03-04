# Live Ops Events Server

## Overview
A Golang backend using Chi for RESTful HTTP APIs and gRPC for internal operations, serving both on the same port.

## Prerequisites
- Go 1.23
- Docker (optional)
- protoc (for regenerating protobuf)

## Building & Running
```bash
# Local build
go build -o liveops ./cmd/server
./liveops

# Docker
docker build -t liveops .
docker run -p 8080:8080 liveops