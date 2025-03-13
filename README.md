# Live Ops Events Server

## Overview

A Golang backend using Chi for RESTful HTTP APIs and gRPC for internal operations, serving both on the same port.

## Prerequisites

- Go 1.23
- Docker (optional)
- protoc (for regenerating protobuf)

## Running

```sh
# launch server
make server
# cli interactive
make cli
# event unit test
# go test -v ./internal/event
# stress client
make stress-test
```

## Scenario
```sh
- make serverd (daemon) | make serve
- make cli 
  - signup (admin / admin-key-456)
  - interact with available functions
- make test (unit test)
- make stress-test 
```

## TODO : Optimisations

```
- [X] use GORM 
- [X] JWT not hardcoded use library
- [X] tweak rate-limit from env
- [X] build protobuf in docker
- [X] env variables from docker compose to *.go
- [] stress client load from env
  - 1 min
    - x 5 sec
      - create 100 event
      - get 50 lists
      - get 1000 events
      - update 20
    - delete all
- [] caching system if event not updated serve cache
```
