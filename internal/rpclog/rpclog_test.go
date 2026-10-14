package rpclog

import (
	"context"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

func TestHandlerLogsCompletedRequest(t *testing.T) {
	var message string
	var fields []any
	handler := Handler{Message: "request completed", Log: func(msg string, args ...any) {
		message, fields = msg, args
	}}
	ctx := handler.TagRPC(context.Background(), &stats.RPCTagInfo{FullMethodName: "/test.Service/Get"})
	start := time.Now()
	handler.HandleRPC(ctx, &stats.End{BeginTime: start, EndTime: start.Add(25 * time.Millisecond), Error: status.Error(codes.NotFound, "missing")})

	want := []any{"method", "/test.Service/Get", "duration_ms", float64(25), "status", "NotFound"}
	if message != "request completed" || !reflect.DeepEqual(fields, want) {
		t.Fatalf("unexpected log: %q %v", message, fields)
	}
}
