package main

import (
    "liveops/internal/server"
    "net"

    "go.uber.org/zap"
)

func main() {
    logger, err := zap.NewProduction()
    if err != nil {
        panic("failed to initialize logger: " + err.Error())
    }
    defer logger.Sync()

    host := "localhost"
    port := "8080"
    url := "http://" + host + ":" + port
    listener, err := net.Listen("tcp", ":"+port)
    if err != nil {
        logger.Fatal("Failed to start listener",
            zap.String("host", host),
            zap.String("port", port),
            zap.Error(err))
    }

    logger.Info("Application started",
        zap.String("host", host),
        zap.String("url", url))

    srv := server.NewServer(logger)
    if err := srv.Serve(listener); err != nil {
        logger.Fatal("Server failed",
            zap.String("host", host),
            zap.String("url", url),
            zap.Error(err))
    }
}