// Package auth verifies credentials used to authenticate API clients.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/metadata"
)

var (
	ErrInvalidBearerToken = errors.New("invalid bearer token")
	ErrInvalidClaims      = errors.New("access token requires a user and positive lifetime")
	ErrInvalidSigningKey  = errors.New("access-token signing key must be at least 32 bytes")
	ErrInvalidToken       = errors.New("invalid access token")
)

type Claims struct {
	Permissions []string `json:"permissions,omitempty"`
	jwt.RegisteredClaims
}

// MakeJWT creates a short-lived internal access token for a user and permissions.
func MakeJWT(userID string, permissions []string, signingKey string, lifetime time.Duration) (string, error) {
	if len(signingKey) < 32 {
		return "", ErrInvalidSigningKey
	}
	if userID == "" || lifetime <= 0 {
		return "", ErrInvalidClaims
	}

	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(lifetime)),
		},
	}).SignedString([]byte(signingKey))
}

// VerifyToken validates an internally issued HS256 access token without a network request.
func VerifyToken(accessToken, signingKey string) (Claims, error) {
	if len(signingKey) < 32 {
		return Claims{}, ErrInvalidSigningKey
	}

	claims := Claims{}
	token, err := jwt.ParseWithClaims(accessToken, &claims, func(*jwt.Token) (any, error) {
		return []byte(signingKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

// GetBearerToken extracts one bearer token from incoming gRPC metadata.
func GetBearerToken(ctx context.Context) (string, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 {
		return "", ErrInvalidBearerToken
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", ErrInvalidBearerToken
	}
	return parts[1], nil
}

// MakeRefreshToken generates a token after the caller has authorized issuance or rotation.
func MakeRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// HasPermission reports whether the signed claims grant an exact permission.
func (claims Claims) HasPermission(permission string) bool {
	if permission == "" {
		return false
	}
	for _, candidate := range claims.Permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}
