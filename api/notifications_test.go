package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/metadata"
)

type testNotificationStore struct {
	message           db.MessageNotification
	friend            db.FriendNotification
	updatedMessage    [2]gocql.UUID
	deletedMessages   []gocql.UUID
	deletedUser       gocql.UUID
	updatedFriend     [2]gocql.UUID
	messageErr        error
	friendErr         error
	messageGets       int
	friendGets        int
	messageAdded      chan struct{}
	friendAdded       chan struct{}
	messageGetStarted chan struct{}
	messageGetRelease <-chan struct{}
	friendGetStarted  chan struct{}
	friendGetRelease  <-chan struct{}
}

type testNotificationMessageStore struct {
	messages map[gocql.UUID]db.DM
}

func (s testNotificationMessageStore) GetMessageByID(_ context.Context, messageID gocql.UUID) (db.DM, error) {
	message, ok := s.messages[messageID]
	if !ok {
		return db.DM{}, gocql.ErrNotFound
	}
	return message, nil
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
	if s.messageGetStarted != nil {
		s.messageGetStarted <- struct{}{}
	}
	if s.messageGetRelease != nil {
		<-s.messageGetRelease
	}
	s.messageGets++
	return []db.MessageNotification{s.message}, nil
}

func (s *testNotificationStore) UpdateMessageNotification(_ context.Context, userID, notificationID gocql.UUID) error {
	s.updatedMessage = [2]gocql.UUID{userID, notificationID}
	return nil
}

func (s *testNotificationStore) DeleteMessageNotifications(_ context.Context, userID gocql.UUID, messageIDs []gocql.UUID) error {
	s.deletedUser = userID
	s.deletedMessages = append(s.deletedMessages, messageIDs...)
	return nil
}

func (s *testNotificationStore) AddFriendNotification(_ context.Context, notification db.FriendNotification) error {
	err := s.friendErr
	s.friend = notification
	if s.friendAdded != nil {
		s.friendAdded <- struct{}{}
	}
	return err
}

type testNotificationStream struct {
	ctx   context.Context
	items []*ntfnV1.SubscribeResponse
}

func (s *testNotificationStream) Send(item *ntfnV1.SubscribeResponse) error {
	s.items = append(s.items, item)
	return nil
}
func (*testNotificationStream) SetHeader(metadata.MD) error  { return nil }
func (*testNotificationStream) SendHeader(metadata.MD) error { return nil }
func (*testNotificationStream) SetTrailer(metadata.MD)       {}
func (s *testNotificationStream) Context() context.Context   { return s.ctx }
func (*testNotificationStream) SendMsg(any) error            { return nil }
func (*testNotificationStream) RecvMsg(any) error            { return nil }

func (s *testNotificationStore) GetFriendNotification(context.Context, gocql.UUID) ([]db.FriendNotification, error) {
	if s.friendGetStarted != nil {
		s.friendGetStarted <- struct{}{}
	}
	if s.friendGetRelease != nil {
		<-s.friendGetRelease
	}
	s.friendGets++
	return []db.FriendNotification{s.friend}, nil
}

type testNotificationBroker struct {
	exchange, routingKey string
	body                 []byte
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
	b.body = body
	return true, b.publishErr
}

func (s *testNotificationStore) UpdateFriendNotification(_ context.Context, userID, notificationID gocql.UUID) error {
	s.updatedFriend = [2]gocql.UUID{userID, notificationID}
	return nil
}

func TestMessageNotificationLifecycle(t *testing.T) {
	userID, messageID, authorID, notificationID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	service := &notificationService{cfg: &Config{logger: slog.Default(), signingKey: apiTestSigningKey}, store: store}

	_, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{
		Id: notificationID.String(), RecipientId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), AuthorName: " Alice ", Content: " hello ", Seen: "true",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if store.message.NotificationID != notificationID || store.message.Seen || store.message.Content != "hello" {
		t.Fatalf("unexpected added notification: %+v", store.message)
	}

	got, err := service.GetMsgN10N(authenticatedTestContext(userID), &ntfnV1.GetMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{}})
	if err != nil || got.Item.Id != store.message.NotificationID.String() || got.Item.Seen != "false" {
		t.Fatalf("unexpected get response: response=%+v error=%v", got, err)
	}
	if _, err := service.UpdateMsgN10N(authenticatedTestContext(userID), &ntfnV1.UpdateMsgN10NRequest{Item: got.Item}); err != nil {
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
		RecipientId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), Content: content,
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
	service := &notificationService{cfg: &Config{logger: slog.Default(), signingKey: apiTestSigningKey}, store: store}

	_, err := service.AddFriendN10N(context.Background(), &ntfnV1.AddFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{
		RecipientId: userID.String(), FriendshipId: friendshipID.String(), FriendId: friendID.String(), FriendName: " Bob ", Status: "pending", Seen: "true",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if store.friend.NotificationID.Version() != 1 || store.friend.Seen || store.friend.FriendName != "Bob" {
		t.Fatalf("unexpected added notification: %+v", store.friend)
	}

	got, err := service.GetFriendN10N(authenticatedTestContext(userID), &ntfnV1.GetFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{}})
	if err != nil || got.Item.Id != store.friend.NotificationID.String() || got.Item.Seen != "false" {
		t.Fatalf("unexpected get response: response=%+v error=%v", got, err)
	}
	if _, err := service.UpdateFriendN10N(authenticatedTestContext(userID), &ntfnV1.UpdateFriendN10NRequest{Item: got.Item}); err != nil {
		t.Fatal(err)
	}
	if store.updatedFriend != [2]gocql.UUID{userID, store.friend.NotificationID} {
		t.Fatalf("unexpected update IDs: %v", store.updatedFriend)
	}
}

func TestSubscribeReturnsUnseenNotifications(t *testing.T) {
	userID := gocql.TimeUUID()
	store := &testNotificationStore{
		message: db.MessageNotification{NotificationID: gocql.TimeUUID(), UserID: userID, MessageID: gocql.TimeUUID(), AuthorID: gocql.TimeUUID()},
		friend:  db.FriendNotification{NotificationID: gocql.TimeUUID(), UserID: userID, FriendshipID: gocql.TimeUUID(), FriendID: gocql.TimeUUID()},
	}
	stream := &testNotificationStream{ctx: authenticatedTestContext(userID)}
	service := &notificationService{cfg: &Config{logger: slog.Default(), signingKey: apiTestSigningKey}, store: store}

	if err := service.Subscribe(&ntfnV1.SubscribeRequest{}, stream); err != nil {
		t.Fatal(err)
	}
	if len(stream.items) != 2 || (stream.items[0].GetMessage() == nil && stream.items[1].GetMessage() == nil) || (stream.items[0].GetFriend() == nil && stream.items[1].GetFriend() == nil) {
		t.Fatalf("unexpected subscribed notifications: %+v", stream.items)
	}
}

func TestReadMessageNotificationIsRemovedWhenAdded(t *testing.T) {
	userID, messageID, authorID, notificationID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	service := &notificationService{
		cfg: &Config{logger: slog.Default()}, store: store,
		messages: testNotificationMessageStore{messages: map[gocql.UUID]db.DM{messageID: {MessageID: messageID, Read: true}}},
	}

	_, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{
		Id: notificationID.String(), RecipientId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if store.updatedMessage != [2]gocql.UUID{userID, notificationID} {
		t.Fatalf("read message notification was retained: %v", store.updatedMessage)
	}
}

func TestSubscribeLoadsNotificationTypesConcurrently(t *testing.T) {
	userID := gocql.TimeUUID()
	messageRelease, friendRelease := make(chan struct{}), make(chan struct{})
	store := &testNotificationStore{
		messageGetStarted: make(chan struct{}, 1), messageGetRelease: messageRelease,
		friendGetStarted: make(chan struct{}, 1), friendGetRelease: friendRelease,
	}
	service := &notificationService{cfg: &Config{logger: slog.Default(), signingKey: apiTestSigningKey}, store: store}
	done := make(chan error, 1)
	go func() {
		done <- service.Subscribe(&ntfnV1.SubscribeRequest{}, &testNotificationStream{ctx: authenticatedTestContext(userID)})
	}()

	for _, started := range []chan struct{}{store.messageGetStarted, store.friendGetStarted} {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(messageRelease)
			close(friendRelease)
			t.Fatal("notification queries did not start concurrently")
		}
	}
	close(messageRelease)
	close(friendRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMessageNotificationUsesTopicAndDatabaseRead(t *testing.T) {
	userID, messageID, authorID, notificationID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	broker := &testNotificationBroker{}
	service := &notificationService{
		cfg:    &Config{logger: slog.Default(), signingKey: apiTestSigningKey},
		store:  store,
		broker: broker,
	}

	item := &ntfnV1.MsgN10NItem{
		Id: notificationID.String(), RecipientId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), AuthorName: "Alice", Content: "hello",
	}
	service.publishMessageNotification(context.Background(), item, slog.Default())
	if _, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: item}); err != nil {
		t.Fatal(err)
	}
	message, err := service.GetMsgN10N(authenticatedTestContext(userID), &ntfnV1.GetMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{}})
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
		cfg:    &Config{logger: slog.Default(), signingKey: apiTestSigningKey},
		store:  store,
		broker: broker,
	}

	_, err := service.AddMsgN10N(context.Background(), &ntfnV1.AddMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{
		RecipientId: userID.String(), MessageId: messageID.String(), AuthorId: authorID.String(), Content: "hello",
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.GetMsgN10N(authenticatedTestContext(userID), &ntfnV1.GetMsgN10NRequest{Item: &ntfnV1.MsgN10NItem{}})
	if err != nil || store.messageGets != 1 || response.GetItem().GetMessageId() != messageID.String() {
		t.Fatalf("database fallback failed: response=%+v gets=%d error=%v", response, store.messageGets, err)
	}
}

func TestSignedOutUserNotificationsUseDatabaseOnly(t *testing.T) {
	userID, friendshipID, friendID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testNotificationStore{}
	broker := &testNotificationBroker{}
	service := &notificationService{
		cfg:    &Config{logger: slog.Default(), signingKey: apiTestSigningKey},
		store:  store,
		broker: broker,
	}

	_, err := service.AddFriendN10N(context.Background(), &ntfnV1.AddFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{
		RecipientId: userID.String(), FriendshipId: friendshipID.String(), FriendId: friendID.String(), Status: "pending",
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.GetFriendN10N(authenticatedTestContext(userID), &ntfnV1.GetFriendN10NRequest{Item: &ntfnV1.FriendN10NItem{}})
	if err != nil || broker.publishes != 0 || store.friendGets != 1 || response.GetItem().GetFriendshipId() != friendshipID.String() {
		t.Fatalf("signed-out user did not use database only: broker=%+v gets=%d response=%+v error=%v", broker, store.friendGets, response, err)
	}
}
