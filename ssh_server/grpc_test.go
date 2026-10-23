package main

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestBearerContext(t *testing.T) {
	metadataValues, ok := metadata.FromOutgoingContext(bearerContext(context.Background(), "internal-token"))
	if !ok || len(metadataValues.Get("authorization")) != 1 || metadataValues.Get("authorization")[0] != "Bearer internal-token" {
		t.Fatalf("unexpected authorization metadata: %v", metadataValues)
	}
}
