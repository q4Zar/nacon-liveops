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
docker compose run --rm -it --build cli   
```

## TODO : Optimisations

```
- use GORM

- JWT not hardcoded

- tweak rate-limit from env

- build protobuf in docker
  
- env variables from docker compose to *.go
- unit
  - auth
  - event

- stress
  - 1 min
    - x 5 sec
      - create 100 event
      - get 50 lists
      - get 1000 events
      - update 20
    - delete all

- caching system if event not updated serve cache
```
