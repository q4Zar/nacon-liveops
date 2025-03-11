package main

import (
	"liveops/internal/server"

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
    addr := ":" + port

    logger.Info("Application started",
        zap.String("host", host),
        zap.String("url", url))

    srv := server.NewServer(logger)
    if err := srv.Start(addr); err != nil {
        logger.Fatal("Server failed",
            zap.String("host", host),
            zap.String("url", url),
            zap.Error(err))
    }
}