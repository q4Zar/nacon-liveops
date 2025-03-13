package main

import (
	"liveops/internal/logger"
	"liveops/internal/server"

	"go.uber.org/zap"
)

func main() {
	logger := logger.NewLogger()

	host := "localhost"
	port := "8080"
	url := "http://" + host + ":" + port

	logger.Info("Application started",
		zap.String("host", host),
		zap.String("url", url))

	srv, err := server.NewServer()
	if err != nil {
		logger.Fatal("Failed to create server",
			zap.String("host", host),
			zap.String("url", url),
			zap.Error(err))
	}

	if err := srv.Start(); err != nil {
		logger.Fatal("Server failed",
			zap.String("host", host),
			zap.String("url", url),
			zap.Error(err))
	}
}
