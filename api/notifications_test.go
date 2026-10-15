package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/gocql/gocql"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/db"
)

type testNotificationStore struct {
	message        db.MessageNotification
	friend         db.FriendNotification
	updatedMessage [2]gocql.UUID
	updatedFriend  [2]gocql.UUID
	messageErr     error
	friendErr      error
}

func (s *testNotificationStore) AddMessageNotification(_ context.Context, notification db.MessageNotification) error {
	s.message = notification
	return s.messageErr
}

func (s *testNotificationStore) GetMessageNotification(context.Context, gocql.UUID) ([]db.MessageNotification, error) {
	return []db.MessageNotification{s.message}, nil
}

func (s *testNotificationStore) UpdateMessageNotification(_ context.Context, userID, notificationID gocql.UUID) error {
	s.updatedMessage = [2]gocql.UUID{userID, notificationID}
	return nil
}

func (s *testNotificationStore) AddFriendNotification(_ context.Context, notification db.FriendNotification) error {
	s.friend = notification
	return s.friendErr
}

func (s *testNotificationStore) GetFriendNotification(context.Context, gocql.UUID) ([]db.FriendNotification, error) {
	return []db.FriendNotification{s.friend}, nil
}

func (s *testNotificationStore) UpdateFriendNotification(_ context.Context, userID, notificationID gocql.UUID) error {
	s.updatedFriend = [2]gocql.UUID{userID, notificationID}
	return nil
}

func TestMessageNotificationLifecycle(t *testing.T) {
	userID, messageID, authorID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	service := &notificationService{cfg: &Config{logger: slog.Default()}, store: store}

	_, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{
		UserId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), AuthorName: " Alice ", Content: " hello ", Seen: "true",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if store.message.NotificationID.Version() != 1 || store.message.Seen || store.message.Content != "hello" {
		t.Fatalf("unexpected added notification: %+v", store.message)
	}

	got, err := service.GetMsgN10N(context.Background(), &ntfnV1.GetMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{UserId: userID.String()}})
	if err != nil || got.Item.Id != store.message.NotificationID.String() || got.Item.Seen != "false" {
		t.Fatalf("unexpected get response: response=%+v error=%v", got, err)
	}
	if _, err := service.UpdateMsgN10N(context.Background(), &ntfnV1.UpdateMsgN10NRequest{Item: got.Item}); err != nil {
		t.Fatal(err)
	}
	if store.updatedMessage != [2]gocql.UUID{userID, store.message.NotificationID} {
		t.Fatalf("unexpected update IDs: %v", store.updatedMessage)
	}
}

func TestAddMessageNotificationTruncatesLongContent(t *testing.T) {
	userID, messageID, authorID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	service := &notificationService{cfg: &Config{logger: slog.Default()}, store: store}
	content := strings.Repeat("🙂", messageNotificationPreviewLength+1)

	_, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{
		UserId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), Content: content,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Repeat("🙂", messageNotificationPreviewLength) + "…"; store.message.Content != want {
		t.Fatalf("unexpected message preview: got %q want %q", store.message.Content, want)
	}
}

func TestFriendNotificationLifecycle(t *testing.T) {
	userID, friendshipID, friendID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	service := &notificationService{cfg: &Config{logger: slog.Default()}, store: store}

	_, err := service.AddFriendN10N(context.Background(), &ntfnV1.AddFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{
		UserId: userID.String(), FriendshipId: friendshipID.String(), FriendId: friendID.String(), FriendName: " Bob ", Status: "pending", Seen: "true",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if store.friend.NotificationID.Version() != 1 || store.friend.Seen || store.friend.FriendName != "Bob" {
		t.Fatalf("unexpected added notification: %+v", store.friend)
	}

	got, err := service.GetFriendN10N(context.Background(), &ntfnV1.GetFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{UserId: userID.String()}})
	if err != nil || got.Item.Id != store.friend.NotificationID.String() || got.Item.Seen != "false" {
		t.Fatalf("unexpected get response: response=%+v error=%v", got, err)
	}
	if _, err := service.UpdateFriendN10N(context.Background(), &ntfnV1.UpdateFriendN10NRequest{Item: got.Item}); err != nil {
		t.Fatal(err)
	}
	if store.updatedFriend != [2]gocql.UUID{userID, store.friend.NotificationID} {
		t.Fatalf("unexpected update IDs: %v", store.updatedFriend)
	}
}
