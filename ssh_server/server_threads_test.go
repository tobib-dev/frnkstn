package main

import (
	"context"
	"testing"

	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type threadLookupClient struct {
	serversv1.ServerServiceClient
}

func (threadLookupClient) GetThreads(context.Context, *serversv1.GetThreadsRequest, ...grpc.CallOption) (*serversv1.GetThreadsResponse, error) {
	return &serversv1.GetThreadsResponse{Items: []*serversv1.ThreadItem{{ThreadId: "thread"}}}, nil
}

func (threadLookupClient) GetThread(context.Context, *serversv1.GetThreadRequest, ...grpc.CallOption) (*serversv1.GetThreadResponse, error) {
	return &serversv1.GetThreadResponse{Items: []*serversv1.ThreadItem{
		{ThreadId: "thread", MessageId: "reply"},
		{ThreadId: "thread", MessageId: "root"},
	}}, nil
}

func TestFindMessageThread(t *testing.T) {
	for _, messageID := range []string{"root", "reply", "unrelated"} {
		t.Run(messageID, func(t *testing.T) {
			id, items, err := findMessageThread(context.Background(), threadLookupClient{}, "server", "channel", messageID)
			if messageID == "unrelated" {
				if status.Code(err) != codes.NotFound {
					t.Fatalf("expected NotFound, got %v", err)
				}
				return
			}
			if err != nil || id != "thread" || len(items) != 2 {
				t.Fatalf("unexpected thread: id=%q items=%v error=%v", id, items, err)
			}
		})
	}
}
