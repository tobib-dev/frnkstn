package main

import (
	"context"

	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func findMessageThread(
	ctx context.Context, client serversv1.ServerServiceClient,
	serverID, channelID, messageID string,
) (string, []*serversv1.ThreadItem, error) {
	threads, err := client.GetThreads(ctx, &serversv1.GetThreadsRequest{
		ServerId: serverID, ChannelId: channelID,
	})
	if err != nil {
		return "", nil, err
	}
	// shortcut: message responses lack thread IDs; add them if lookup cost grows with thread count.
	seen := make(map[string]bool)
	for _, thread := range threads.GetItems() {
		threadID := thread.GetThreadId()
		if seen[threadID] {
			continue
		}
		seen[threadID] = true
		response, err := client.GetThread(ctx, &serversv1.GetThreadRequest{
			ServerId: serverID, ChannelId: channelID, ThreadId: threadID,
		})
		if err != nil {
			return "", nil, err
		}
		for _, item := range response.GetItems() {
			if item.GetMessageId() == messageID {
				return threadID, response.GetItems(), nil
			}
		}
	}
	return "", nil, status.Error(codes.NotFound, "message thread not found")
}
