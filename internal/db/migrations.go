package db

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// MigrateDB handles all database migrations
func MigrateDB(db *gorm.DB) error {
	log.Println("Running database migrations...")

	// Create tables
	if err := createTables(db); err != nil {
		return fmt.Errorf("failed to create tables: %v", err)
	}

	// Populate initial data
	if err := populateInitialData(db); err != nil {
		return fmt.Errorf("failed to populate initial data: %v", err)
	}

	log.Println("Database migrations completed successfully")
	return nil
}

func createTables(db *gorm.DB) error {
	// Create users table with indexes
	if err := db.AutoMigrate(&User{}); err != nil {
		return fmt.Errorf("failed to migrate users table: %v", err)
	}

	// Create events table with indexes
	if err := db.AutoMigrate(&Event{}); err != nil {
		return fmt.Errorf("failed to migrate events table: %v", err)
	}

	return nil
}

func populateInitialData(db *gorm.DB) error {
	// Check if default users already exist
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count users: %v", err)
	}

	// Only create default users if no users exist
	if count == 0 {
		// Create public user
		publicPass, err := bcrypt.GenerateFromPassword([]byte("public-key-123"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash public user password: %v", err)
		}

		publicUser := User{
			ID:        GenerateID(),
			Username:  "public",
			Password:  string(publicPass),
			Type:      UserTypeHTTP,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(&publicUser).Error; err != nil {
			return fmt.Errorf("failed to create public user: %v", err)
		}

		// Create admin user
		adminPass, err := bcrypt.GenerateFromPassword([]byte("admin-key-456"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash admin user password: %v", err)
		}

		adminUser := User{
			ID:        GenerateID(),
			Username:  "admin",
			Password:  string(adminPass),
			Type:      UserTypeAdmin,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(&adminUser).Error; err != nil {
			return fmt.Errorf("failed to create admin user: %v", err)
		}

		log.Println("Created default users: public and admin")
	}

	return nil
}
