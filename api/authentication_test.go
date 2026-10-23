package main

import (
	"context"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/tobib-dev/frnkstn/api/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const apiTestSigningKey = "01234567890123456789012345678901"

func authenticatedTestContext(userID gocql.UUID) context.Context {
	token, err := auth.MakeJWT(userID.String(), auth.DefaultUserPermissions(), apiTestSigningKey, time.Hour)
	if err != nil {
		panic(err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
}

func TestAuthenticatedUserID(t *testing.T) {
	userID := gocql.TimeUUID()
	for _, test := range []struct {
		name        string
		permissions []string
		withToken   bool
		wantCode    codes.Code
	}{
		{name: "valid", permissions: []string{auth.PermissionReadMessages}, withToken: true},
		{name: "missing permission", withToken: true, wantCode: codes.PermissionDenied},
		{name: "missing token", wantCode: codes.Unauthenticated},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.withToken {
				token, err := auth.MakeJWT(userID.String(), test.permissions, apiTestSigningKey, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
			}
			got, err := authenticatedUserID(ctx, &Config{signingKey: apiTestSigningKey}, auth.PermissionReadMessages)
			if status.Code(err) != test.wantCode || (err == nil && got != userID) {
				t.Fatalf("authenticatedUserID() = %v, %v; want user %v, code %v", got, err, userID, test.wantCode)
			}
		})
	}
}
