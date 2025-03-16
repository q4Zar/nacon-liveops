# Live Ops Events Server

## Overview

A Golang backend using Chi for RESTful HTTP APIs and gRPC for internal operations, serving both on the same port.

## Prerequisites

- Go 1.23
- Docker | Docker-Compose

## Scenario
```sh
- cp .env.example .env
- make serverd (daemon) | make serve
- make test (unit test)
- make stress-test (use admin / admin-key-456 by default)
- make cli 
  - signup | signin
  - interact with available functions
```

thanks for reading | using ;)
