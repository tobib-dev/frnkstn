package main

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/tobib-dev/frnkstn/api/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func authenticatedUserID(ctx context.Context, cfg *Config, permission string) (gocql.UUID, error) {
	token, err := auth.GetBearerToken(ctx)
	if err != nil {
		return gocql.UUID{}, status.Error(codes.Unauthenticated, "bearer token is required")
	}
	claims, err := auth.VerifyToken(token, cfg.signingKey)
	if err != nil {
		return gocql.UUID{}, status.Error(codes.Unauthenticated, "invalid bearer token")
	}
	if permission != "" && !claims.HasPermission(permission) {
		return gocql.UUID{}, status.Error(codes.PermissionDenied, "permission denied")
	}
	userID, err := gocql.ParseUUID(claims.Subject)
	if err != nil {
		return gocql.UUID{}, status.Error(codes.Unauthenticated, "invalid bearer token subject")
	}
	return userID, nil
}
