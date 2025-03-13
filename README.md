# Live Ops Events Server

## Overview

A Golang backend using Chi for RESTful HTTP APIs and gRPC for internal operations, serving both on the same port.

## Prerequisites

- Go 1.23
- Docker (optional)
- protoc (for regenerating protobuf)

## Scenario
```sh
- cp .env.example .env
- make serverd (daemon) | make serve
- make cli 
  - signup (admin / admin-key-456)
  - interact with available functions
- make test (unit test)
- make stress-test (absolutely needs admin / admin-key-456 created ... could have been done with seed file ikik)
```

thanks for using ;)
