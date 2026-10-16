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
	messageGets    int
	friendGets     int
	messageAdded   chan struct{}
}

func (s *testNotificationStore) AddMessageNotification(_ context.Context, notification db.MessageNotification) error {
	err := s.messageErr
	s.message = notification
	if s.messageAdded != nil {
		s.messageAdded <- struct{}{}
	}
	return err
}

func (s *testNotificationStore) GetMessageNotification(context.Context, gocql.UUID) ([]db.MessageNotification, error) {
	s.messageGets++
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
	s.friendGets++
	return []db.FriendNotification{s.friend}, nil
}

type testNotificationBroker struct {
	exchange, routingKey string
	publishes            int
	publishErr           error
	publishStarted       chan struct{}
	publishRelease       <-chan struct{}
	publishFinished      chan struct{}
}

func (b *testNotificationBroker) Publish(_ context.Context, exchange, routingKey string, body []byte) (bool, error) {
	if b.publishFinished != nil {
		defer func() { b.publishFinished <- struct{}{} }()
	}
	if b.publishStarted != nil {
		b.publishStarted <- struct{}{}
	}
	if b.publishRelease != nil {
		<-b.publishRelease
	}
	b.publishes++
	b.exchange, b.routingKey = exchange, routingKey
	return true, b.publishErr
}

func (s *testNotificationStore) UpdateFriendNotification(_ context.Context, userID, notificationID gocql.UUID) error {
	s.updatedFriend = [2]gocql.UUID{userID, notificationID}
	return nil
}

func TestMessageNotificationLifecycle(t *testing.T) {
	userID, messageID, authorID, notificationID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	service := &notificationService{cfg: &Config{logger: slog.Default()}, store: store}

	_, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{
		Id: notificationID.String(), UserId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), AuthorName: " Alice ", Content: " hello ", Seen: "true",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if store.message.NotificationID != notificationID || store.message.Seen || store.message.Content != "hello" {
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

func TestMessageNotificationUsesTopicAndDatabaseRead(t *testing.T) {
	userID, messageID, authorID, notificationID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	broker := &testNotificationBroker{}
	service := &notificationService{
		cfg:    &Config{logger: slog.Default()},
		store:  store,
		broker: broker,
	}

	item := &ntfnV1.MsgN10NItem{
		Id: notificationID.String(), UserId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), AuthorName: "Alice", Content: "hello",
	}
	service.publishMessageNotification(context.Background(), item, slog.Default())
	if _, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: item}); err != nil {
		t.Fatal(err)
	}
	message, err := service.GetMsgN10N(context.Background(), &ntfnV1.GetMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{UserId: userID.String()}})
	if err != nil {
		t.Fatal(err)
	}
	if broker.exchange != notificationExchange || broker.routingKey != "notifications."+userID.String() || broker.publishes != 1 || store.messageGets != 1 || message.GetItem().GetId() != notificationID.String() {
		t.Fatalf("message notification did not use its topic route: broker=%+v gets=%d item=%+v", broker, store.messageGets, message.GetItem())
	}
}

func TestGetMessageNotificationReadsDatabaseOnly(t *testing.T) {
	userID, messageID, authorID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	broker := &testNotificationBroker{}
	service := &notificationService{
		cfg:    &Config{logger: slog.Default()},
		store:  store,
		broker: broker,
	}

	_, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{
		UserId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), Content: "hello",
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.GetMsgN10N(context.Background(), &ntfnV1.GetMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{UserId: userID.String()}})
	if err != nil || store.messageGets != 1 || response.GetItem().GetMessageId() != messageID.String() {
		t.Fatalf("database fallback failed: response=%+v gets=%d error=%v", response, store.messageGets, err)
	}
}

func TestSignedOutUserNotificationsUseDatabaseOnly(t *testing.T) {
	userID, friendshipID, friendID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	broker := &testNotificationBroker{}
	service := &notificationService{
		cfg:    &Config{logger: slog.Default()},
		store:  store,
		broker: broker,
	}

	_, err := service.AddFriendN10N(context.Background(), &ntfnV1.AddFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{
		UserId: userID.String(), FriendshipId: friendshipID.String(), FriendId: friendID.String(), Status: "pending",
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.GetFriendN10N(context.Background(), &ntfnV1.GetFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{UserId: userID.String()}})
	if err != nil || broker.publishes != 0 || store.friendGets != 1 || response.GetItem().GetFriendshipId() != friendshipID.String() {
		t.Fatalf("signed-out user did not use database only: broker=%+v gets=%d response=%+v error=%v", broker, store.friendGets, response, err)
	}
}
