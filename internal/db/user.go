package db

import (
	"errors"
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
	// Auto migrate the user table
	if err := db.AutoMigrate(&User{}); err != nil {
		panic(err)
	}
	return &userRepository{db: db}
}

func (r *userRepository) CreateUser(username, password string, userType UserType) error {
	// Check if user already exists
	var count int64
	if err := r.db.Model(&User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("username already exists")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := User{
		ID:       GenerateID(),
		Username: username,
		Password: string(hashedPassword),
		Type:     userType,
	}

	return r.db.Create(&user).Error
}

func (r *userRepository) ValidateUser(username, password string) (*User, error) {
	var user User
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid credentials")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	return &user, nil
}

func (r *userRepository) GetUser(username string) (*User, error) {
	var user User
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
} 