package main

import (
    "liveops/internal/server"
    "log"
    "net"
)

func main() {
    listener, err := net.Listen("tcp", ":8080")
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    srv := server.NewServer()
    if err := srv.Serve(listener); err != nil {
        log.Fatalf("server failed: %v", err)
    }
}