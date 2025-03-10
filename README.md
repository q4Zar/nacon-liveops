# Live Ops Events Server

## Overview

A Golang backend using Chi for RESTful HTTP APIs and gRPC for internal operations, serving both on the same port.

## Prerequisites

- Go 1.23
- Docker (optional)
- protoc (for regenerating protobuf)

## Running

```sh
# server as daemon
docker compose up --build -d go-nacon
# event unit test
go test -v ./internal/event
# stress client
docker compose up --build stressclient
# cli interactive
docker compose run --rm -it cli
```

## TODO : Optimisations

```
- JWT not hardcoded
- use GORM
- stressclient (balance tests gRPC & HTTP)
- tweak rate-limit from env
- build protobuf in docker
```
