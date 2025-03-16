package db

import (
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type DB struct {
	*gorm.DB
}

func NewDB(dbPath string) (*DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Run migrations
	if err := MigrateDB(db); err != nil {
		return nil, err
	}

	return &DB{DB: db}, nil
}

func GenerateID() string {
	return uuid.New().String()
}
