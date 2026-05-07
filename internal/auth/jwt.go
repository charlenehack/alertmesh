package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Username string   `json:"username"`
	UserID   string   `json:"user_id"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secret      []byte
	expiryHours int
}

func NewJWTService(secret string, expiryHours int) *JWTService {
	return &JWTService{
		secret:      []byte(secret),
		expiryHours: expiryHours,
	}
}

func (s *JWTService) GenerateToken(userID, username string, roles []string) (string, error) {
	claims := &Claims{
		Username: username,
		UserID:   userID,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "alertmesh",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *JWTService) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
