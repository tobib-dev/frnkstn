package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/metadata"
)

const testSigningKey = "01234567890123456789012345678901"

func TestMakeJWT(t *testing.T) {
	token, err := MakeJWT("user-id", []string{"messages:read"}, testSigningKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := VerifyToken(token, testSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-id" || !claims.HasPermission("messages:read") || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	for _, test := range []struct {
		name, userID, key string
		lifetime          time.Duration
		want              error
	}{
		{name: "missing user", key: testSigningKey, lifetime: time.Hour, want: ErrInvalidClaims},
		{name: "invalid lifetime", userID: "user-id", key: testSigningKey, want: ErrInvalidClaims},
		{name: "short key", userID: "user-id", key: "short", lifetime: time.Hour, want: ErrInvalidSigningKey},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := MakeJWT(test.userID, nil, test.key, test.lifetime)
			if !errors.Is(err, test.want) {
				t.Fatalf("MakeJWT() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestVerifyToken(t *testing.T) {
	valid := Claims{
		Permissions: []string{"messages:read"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-id",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	tests := []struct {
		name   string
		claims Claims
		method jwt.SigningMethod
		key    string
		want   error
	}{
		{name: "valid", claims: valid, method: jwt.SigningMethodHS256, key: testSigningKey},
		{name: "expired", claims: Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user-id", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))}}, method: jwt.SigningMethodHS256, key: testSigningKey, want: ErrInvalidToken},
		{name: "missing expiration", claims: Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user-id"}}, method: jwt.SigningMethodHS256, key: testSigningKey, want: ErrInvalidToken},
		{name: "missing user", claims: Claims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}, method: jwt.SigningMethodHS256, key: testSigningKey, want: ErrInvalidToken},
		{name: "wrong signature", claims: valid, method: jwt.SigningMethodHS256, key: "abcdefghijklmnopqrstuvwxyz123456", want: ErrInvalidToken},
		{name: "wrong algorithm", claims: valid, method: jwt.SigningMethodHS384, key: testSigningKey, want: ErrInvalidToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signed, err := jwt.NewWithClaims(tt.method, tt.claims).SignedString([]byte(testSigningKey))
			if err != nil {
				t.Fatal(err)
			}
			claims, err := VerifyToken(signed, tt.key)
			if !errors.Is(err, tt.want) {
				t.Fatalf("VerifyToken() error = %v, want %v", err, tt.want)
			}
			if tt.want == nil && (claims.Subject != "user-id" || !claims.HasPermission("messages:read") || claims.HasPermission("messages:write")) {
				t.Fatalf("unexpected claims: %+v", claims)
			}
		})
	}
}

func TestVerifyTokenRejectsShortSigningKey(t *testing.T) {
	_, err := VerifyToken("token", "short")
	if !errors.Is(err, ErrInvalidSigningKey) {
		t.Fatalf("VerifyToken() error = %v, want ErrInvalidSigningKey", err)
	}
}

func TestGetBearerToken(t *testing.T) {
	for _, test := range []struct {
		name   string
		values []string
		want   string
		ok     bool
	}{
		{name: "valid", values: []string{"Bearer token"}, want: "token", ok: true},
		{name: "case insensitive scheme", values: []string{"bearer token"}, want: "token", ok: true},
		{name: "missing"},
		{name: "wrong scheme", values: []string{"Basic token"}},
		{name: "missing token", values: []string{"Bearer"}},
		{name: "extra fields", values: []string{"Bearer token extra"}},
		{name: "duplicate", values: []string{"Bearer first", "Bearer second"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{"authorization": test.values})
			got, err := GetBearerToken(ctx)
			if test.ok && (err != nil || got != test.want) {
				t.Fatalf("GetBearerToken() = %q, %v; want %q", got, err, test.want)
			}
			if !test.ok && !errors.Is(err, ErrInvalidBearerToken) {
				t.Fatalf("GetBearerToken() error = %v, want ErrInvalidBearerToken", err)
			}
		})
	}
}

func TestMakeRefreshToken(t *testing.T) {
	first, err := MakeRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := MakeRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := hex.DecodeString(first)
	if err != nil || len(decoded) != 32 || first == second {
		t.Fatalf("invalid refresh tokens: first=%q second=%q error=%v", first, second, err)
	}
}
