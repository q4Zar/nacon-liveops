package main

import (
	"liveops/internal/server"
	"net"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
    ID       uint   `gorm:"primaryKey"`
    Username string `gorm:"unique;not null"`
    Key      string `gorm:"unique;not null"`
    Type     string `gorm:"not null;check:type IN ('admin', 'public')"`
}

func main() {
    logger, err := zap.NewProduction()
    if err != nil {
        panic("failed to initialize logger: " + err.Error())
    }
    defer logger.Sync()

    // Initialize GORM with SQLite
    db, err := gorm.Open(sqlite.Open("./db/users.db"), &gorm.Config{})
    if err != nil {
        logger.Fatal("Failed to open database", zap.Error(err))
    }

    // Auto-migrate the users table
    if err := db.AutoMigrate(&User{}); err != nil {
        logger.Fatal("Failed to migrate database", zap.Error(err))
    }

    // Seed initial users
    if err := seedUsers(db, logger); err != nil {
        logger.Fatal("Failed to seed users", zap.Error(err))
    }

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

    srv := server.NewServer(logger, db)
    if err := srv.Serve(listener); err != nil {
        logger.Fatal("Server failed",
            zap.String("host", host),
            zap.String("url", url),
            zap.Error(err))
    }
}

// seedUsers initializes the users table with default users
func seedUsers(db *gorm.DB, logger *zap.Logger) error {
    defaultUsers := []User{
        {Username: "admin1", Key: "admin-key-456", Type: "admin"},
        {Username: "public1", Key: "public-key-123", Type: "public"},
    }

    for _, user := range defaultUsers {
        var existing User
        if err := db.Where("username = ?", user.Username).First(&existing).Error; err != nil {
            if err == gorm.ErrRecordNotFound {
                if err := db.Create(&user).Error; err != nil {
                    logger.Error("Failed to seed user",
                        zap.String("username", user.Username),
                        zap.Error(err))
                    return err
                }
                logger.Info("Seeded user",
                    zap.String("username", user.Username),
                    zap.String("type", user.Type))
            } else {
                logger.Error("Failed to check existing user",
                    zap.String("username", user.Username),
                    zap.Error(err))
                return err
            }
        }
    }
    return nil
}