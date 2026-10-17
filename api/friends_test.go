package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/gocql/gocql"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/protobuf/proto"
)

type testFriendStore struct {
	friend           db.Friend
	rejected         db.Friend
	added            int
	removedID        gocql.UUID
	removedForUser   gocql.UUID
	removedForFriend gocql.UUID
	addedUserName    string
	getFriendsCalls  int
	listed           []db.Friend
}

func (s *testFriendStore) AddFriend(_ context.Context, params db.AddFriendParams) (db.Friend, error) {
	s.added++
	s.addedUserName = params.RequesterUsername
	friend := params.Friend
	friend.Status = "pending"
	s.friend = friend
	return friend, nil
}

func (s *testFriendStore) AcceptFriend(_ context.Context, friend db.Friend) (db.Friend, error) {
	friend.Status = "accepted"
	return friend, nil
}

func (s *testFriendStore) RejectFriend(_ context.Context, friend db.Friend) (db.Friend, error) {
	friend.Status = "rejected"
	s.rejected = friend
	return friend, nil
}

func (s *testFriendStore) RemoveFriend(_ context.Context, friendshipID, userID, friendID gocql.UUID) error {
	s.removedID = friendshipID
	s.removedForUser = userID
	s.removedForFriend = friendID
	return nil
}
func (s *testFriendStore) GetFriend(_ context.Context, id gocql.UUID) (db.Friend, error) {
	if s.friend.ID != id {
		return db.Friend{}, gocql.ErrNotFound
	}
	return s.friend, nil
}
func (s *testFriendStore) GetFriends(_ context.Context, userID gocql.UUID) ([]db.Friend, error) {
	s.getFriendsCalls++
	if s.listed != nil {
		return s.listed, nil
	}
	if s.friend.UserID != userID {
		return nil, nil
	}
	return []db.Friend{s.friend}, nil
}

func TestGetFriendsIdentifiesRequester(t *testing.T) {
	requester, recipient, friendshipID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testFriendStore{
		friend: db.Friend{ID: friendshipID, UserID: requester, FriendID: recipient, FriendName: "recipient", Status: "pending"},
		listed: []db.Friend{{ID: friendshipID, UserID: recipient, FriendID: requester, FriendName: "requester", Status: "pending"}},
	}
	service := &FriendService{cfg: &Config{logger: slog.Default()}, store: store}

	response, err := service.GetFriends(context.Background(), &friendsv1.GetFriendsRequest{UserId: recipient.String()})
	if err != nil {
		t.Fatal(err)
	}
	item := response.Items[0]
	if item.UserId != requester.String() || item.FriendId != recipient.String() || item.FriendUsername != "requester" {
		t.Fatalf("unexpected recipient view: %+v", item)
	}
}

func TestRemoveFriendUsesFriendshipID(t *testing.T) {
	friendship := db.Friend{ID: gocql.TimeUUID(), UserID: gocql.TimeUUID(), FriendID: gocql.TimeUUID()}
	store := &testFriendStore{friend: friendship}
	service := &FriendService{cfg: &Config{logger: slog.Default()}, store: store}

	response, err := service.RemoveFriend(context.Background(), &friendsv1.RemoveFriendRequest{
		FriendshipId: friendship.ID.String(),
		UserId:       friendship.UserID.String(),
		FriendId:     friendship.FriendID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "removed" {
		t.Fatalf("unexpected status: %q", response.Status)
	}
	if store.removedID != friendship.ID || store.removedForUser != friendship.UserID || store.removedForFriend != friendship.FriendID {
		t.Fatalf("unexpected remove arguments: friendship=%s user=%s friend=%s", store.removedID, store.removedForUser, store.removedForFriend)
	}
	if store.getFriendsCalls != 0 {
		t.Fatalf("RemoveFriend listed friends %d times", store.getFriendsCalls)
	}
}

func TestRejectFriendUsesFriendshipID(t *testing.T) {
	friendship := db.Friend{ID: gocql.TimeUUID(), UserID: gocql.TimeUUID(), FriendID: gocql.TimeUUID()}
	store := &testFriendStore{friend: friendship}
	service := &FriendService{cfg: &Config{logger: slog.Default()}, store: store}

	response, err := service.RejectFriend(context.Background(), &friendsv1.RejectFriendRequest{
		FriendshipId: friendship.ID.String(),
		UserId:       friendship.FriendID.String(),
		FriendId:     friendship.UserID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.FriendshipId != friendship.ID.String() || response.Status != "rejected" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if store.rejected.ID != friendship.ID {
		t.Fatalf("unexpected rejected friendship: %+v", store.rejected)
	}
	if store.getFriendsCalls != 0 {
		t.Fatalf("RejectFriend listed friends %d times", store.getFriendsCalls)
	}
}

func TestAddFriendGeneratesTimeUUIDOnServer(t *testing.T) {
	userID, friendID := gocql.TimeUUID(), gocql.TimeUUID()
	users := &testUserStore{users: map[gocql.UUID]db.User{
		userID:   {ID: userID, Username: "alice"},
		friendID: {ID: friendID, Username: "bob"},
	}}
	store := &testFriendStore{}
	notifications := &testNotificationStore{friendAdded: make(chan struct{}, 1)}
	cfg := &Config{logger: slog.Default()}
	service := &FriendService{cfg: cfg, store: store, users: users, n10n: &notificationService{cfg: cfg, store: notifications}}

	response, err := service.AddFriend(context.Background(), &friendsv1.AddFriendRequest{UserId: userID.String(), FriendId: friendID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if store.added != 1 || store.friend.ID == (gocql.UUID{}) || store.friend.ID.Version() != 1 {
		t.Fatalf("friendship ID was not generated by the service: %+v", store.friend)
	}
	if response.FriendshipId != store.friend.ID.String() || response.Status != "pending" || store.friend.FriendName != "bob" || store.addedUserName != "alice" {
		t.Fatalf("unexpected response or stored friend: response=%+v friend=%+v", response, store.friend)
	}
	select {
	case <-notifications.friendAdded:
	case <-time.After(time.Second):
		t.Fatal("friend notification was not added asynchronously")
	}
	if notification := notifications.friend; notification.UserID != friendID || notification.FriendshipID != store.friend.ID || notification.FriendID != userID || notification.FriendName != "alice" || notification.Status != "pending" {
		t.Fatalf("unexpected friend notification: %+v", notification)
	}

	notifications.friendErr = errors.New("notification unavailable")
	if _, err := service.AddFriend(context.Background(), &friendsv1.AddFriendRequest{UserId: userID.String(), FriendId: friendID.String()}); err != nil {
		t.Fatalf("notification failure changed successful friend request: %v", err)
	}
	select {
	case <-notifications.friendAdded:
	case <-time.After(time.Second):
		t.Fatal("failed friend notification was not attempted")
	}
}

func TestAddFriendDoesNotWaitForNotificationPublish(t *testing.T) {
	userID, friendID := gocql.TimeUUID(), gocql.TimeUUID()
	release := make(chan struct{})
	broker := &testNotificationBroker{publishStarted: make(chan struct{}, 1), publishRelease: release, publishFinished: make(chan struct{}, 1)}
	service := &FriendService{
		cfg:   &Config{logger: slog.Default()},
		store: &testFriendStore{},
		users: &testUserStore{users: map[gocql.UUID]db.User{userID: {ID: userID, Username: "alice"}, friendID: {ID: friendID, Username: "bob"}}},
		n10n:  &notificationService{cfg: &Config{logger: slog.Default()}, store: &testNotificationStore{}, broker: broker},
	}
	done := make(chan error, 1)
	go func() {
		_, err := service.AddFriend(context.Background(), &friendsv1.AddFriendRequest{UserId: userID.String(), FriendId: friendID.String()})
		done <- err
	}()

	select {
	case <-broker.publishStarted:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("friend notification publish did not start")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("friend request waited for notification publish")
	}
	close(release)
	select {
	case <-broker.publishFinished:
	case <-time.After(time.Second):
		t.Fatal("friend notification publish did not finish")
	}
	envelope := &ntfnV1.SubscribeResponse{}
	if err := proto.Unmarshal(broker.body, envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.GetFriend() == nil || envelope.GetFriend().GetUserId() != friendID.String() || broker.routingKey != "notifications."+friendID.String() {
		t.Fatalf("unexpected friend notification publish: broker=%+v item=%+v", broker, envelope.GetFriend())
	}
}
