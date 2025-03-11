package db

import (
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserType string

const (
	UserTypeHTTP  UserType = "http"
	UserTypeAdmin UserType = "admin"
)

type User struct {
	ID        string    `gorm:"primaryKey"`
	Username  string    `gorm:"uniqueIndex;not null"`
	Password  string    `gorm:"not null"`
	Type      UserType  `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

type UserRepository interface {
	CreateUser(username, password string, userType UserType) error
	ValidateUser(username, password string) (*User, error)
	GetUser(username string) (*User, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	log.Println("Initializing user repository...")
	// Auto migrate the user table
	if err := db.AutoMigrate(&User{}); err != nil {
		log.Printf("Failed to auto-migrate user table: %v", err)
		panic(err)
	}
	log.Println("User table migration completed successfully")
	return &userRepository{db: db}
}

func (r *userRepository) CreateUser(username, password string, userType UserType) error {
	log.Printf("Attempting to create user: %s (type: %s)", username, userType)

	// Check if user already exists
	var count int64
	if err := r.db.Model(&User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		log.Printf("Error checking for existing user: %v", err)
		return fmt.Errorf("database error: %v", err)
	}
	if count > 0 {
		log.Printf("User %s already exists", username)
		return errors.New("username already exists")
	}

	// Hash password
	log.Println("Hashing password...")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		return fmt.Errorf("failed to hash password: %v", err)
	}

	user := User{
		ID:       GenerateID(),
		Username: username,
		Password: string(hashedPassword),
		Type:     userType,
	}

	log.Printf("Creating user in database with ID: %s", user.ID)
	if err := r.db.Create(&user).Error; err != nil {
		log.Printf("Error creating user in database: %v", err)
		return fmt.Errorf("failed to create user: %v", err)
	}

	log.Printf("User %s created successfully", username)
	return nil
}

func (r *userRepository) ValidateUser(username, password string) (*User, error) {
	log.Printf("Attempting to validate user: %s", username)

	var user User
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("User %s not found", username)
			return nil, errors.New("invalid credentials")
		}
		log.Printf("Database error while validating user: %v", err)
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		log.Printf("Invalid password for user: %s", username)
		return nil, errors.New("invalid credentials")
	}

	log.Printf("User %s validated successfully", username)
	return &user, nil
}

func (r *userRepository) GetUser(username string) (*User, error) {
	log.Printf("Looking up user: %s", username)

	var user User
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("User %s not found", username)
			return nil, errors.New("user not found")
		}
		log.Printf("Database error while getting user: %v", err)
		return nil, err
	}

	log.Printf("User %s found", username)
	return &user, nil
} 