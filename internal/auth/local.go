package auth

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

type LocalAuth struct {
	db *gorm.DB
}

func NewLocalAuth(db *gorm.DB) *LocalAuth {
	return &LocalAuth{db: db}
}

func (a *LocalAuth) Authenticate(ctx context.Context, username, password string) (*model.User, error) {
	var user model.User
	if err := a.db.WithContext(ctx).Preload("Roles").Where("username = ? AND source = ?", username, "local").First(&user).Error; err != nil {
		return nil, errors.New("invalid credentials")
	}
	if !user.IsActive {
		return nil, errors.New("account disabled")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return &user, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
