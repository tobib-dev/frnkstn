package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gocql/gocql"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FriendService struct {
	cfg   *Config
	store db.FriendStore
	users db.UserStore
	n10n  *notificationService
	friendsv1.UnimplementedFriendServiceServer
}

func NewFriendService(cfg *Config) *FriendService {
	return &FriendService{cfg: cfg, store: &cfg.db.session, users: &cfg.db.session, n10n: NewNotificationService(cfg)}
}

func (s *FriendService) AddFriend(ctx context.Context, req *friendsv1.AddFriendRequest) (*friendsv1.AddFriendResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger := s.cfg.logger.With("user_id", userID, "friend_id", req.FriendId)
	logger.Info("adding friend")

	_, friendID, err := friendIDs(userID.String(), req.FriendId)
	if err != nil {
		logger.Warn("invalid add friend request", "error", err)
		return nil, err
	}
	if userID == friendID {
		logger.Warn("cannot add user as their own friend")
		return nil, status.Error(codes.InvalidArgument, "cannot add yourself as a friend")
	}
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		logger.Error("failed to get requesting user", "error", err)
		return nil, userLookupError(err)
	}
	friendUser, err := s.users.GetUserByID(ctx, friendID)
	if err != nil {
		logger.Error("failed to get requested friend", "error", err)
		return nil, userLookupError(err)
	}

	friendship := db.Friend{ID: gocql.TimeUUID(), UserID: userID, FriendID: friendID, FriendName: friendUser.Username}
	friendship, err = s.store.AddFriend(ctx, db.AddFriendParams{Friend: friendship, RequesterUsername: user.Username})
	if err != nil {
		logger.Error("failed to add friend", "error", err, "friendship_id", friendship.ID)
		return nil, status.Error(codes.Internal, "could not add friend")
	}
	s.publishFriendNotification(friendship, user.Username, logger)
	logger.Info("friend added", "friendship_id", friendship.ID, "status", friendship.Status)
	return &friendsv1.AddFriendResponse{FriendshipId: friendship.ID.String(), FriendId: friendship.FriendID.String(), Status: friendship.Status}, nil
}

func (s *FriendService) publishFriendNotification(friendship db.Friend, requesterName string, logger *slog.Logger) {
	// shortcut: asynchronous publish can be lost during process shutdown; use an outbox when delivery must survive crashes.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.n10n.publishFriendNotification(ctx, &ntfnV1.FriendN10NItem{
			Id:           gocql.TimeUUID().String(),
			RecipientId:  friendship.FriendID.String(),
			FriendshipId: friendship.ID.String(),
			FriendId:     friendship.UserID.String(),
			FriendName:   requesterName,
			Status:       friendship.Status,
		}, logger.With("friendship_id", friendship.ID.String()))
	}()
}

func (s *FriendService) AcceptFriend(ctx context.Context, req *friendsv1.AcceptFriendRequest) (*friendsv1.AcceptFriendResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger := s.cfg.logger.With("friendship_id", req.FriendshipId, "user_id", userID, "friend_id", req.FriendId)
	logger.Info("accepting friend")
	friendship, err := s.friendship(ctx, req.FriendshipId, userID.String(), req.FriendId)
	if err != nil {
		logger.Error("failed to get friendship", "error", err)
		return nil, err
	}
	if friendship.FriendID != userID {
		return nil, status.Error(codes.PermissionDenied, "only the recipient can accept a friend request")
	}
	friendship, err = s.store.AcceptFriend(ctx, friendship)
	if err != nil {
		logger.Error("failed to accept friend", "error", err)
		return nil, status.Error(codes.Internal, "could not accept friend request")
	}
	logger.Info("friend accepted", "status", friendship.Status)
	return &friendsv1.AcceptFriendResponse{FriendshipId: friendship.ID.String(), UserId: friendship.UserID.String(), FriendId: friendship.FriendID.String(), Status: friendship.Status}, nil
}

func (s *FriendService) RejectFriend(ctx context.Context, req *friendsv1.RejectFriendRequest) (*friendsv1.RejectFriendResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger := s.cfg.logger.With("friendship_id", req.FriendshipId, "user_id", userID, "friend_id", req.FriendId)
	logger.Info("rejecting friend")
	friendship, err := s.friendship(ctx, req.FriendshipId, userID.String(), req.FriendId)
	if err != nil {
		logger.Error("failed to get friendship", "error", err)
		return nil, err
	}
	if friendship.FriendID != userID {
		return nil, status.Error(codes.PermissionDenied, "only the recipient can decline a friend request")
	}
	friendship, err = s.store.RejectFriend(ctx, friendship)
	if err != nil {
		logger.Error("failed to reject friend", "error", err)
		return nil, status.Error(codes.Internal, "could not reject friend request")
	}
	logger.Info("friend rejected", "status", friendship.Status)
	return &friendsv1.RejectFriendResponse{FriendshipId: friendship.ID.String(), UserId: friendship.UserID.String(), FriendId: friendship.FriendID.String(), Status: friendship.Status}, nil
}

func (s *FriendService) GetFriends(ctx context.Context, req *friendsv1.GetFriendsRequest) (*friendsv1.GetFriendsResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	friends, err := s.store.GetFriends(ctx, userID)
	if err != nil {
		s.cfg.logger.Error("failed to get friends", "error", err, "user_id", userID)
		return nil, status.Error(codes.Internal, "could not get friends")
	}
	items := make([]*friendsv1.GetFriendsItem, 0, len(friends))
	for _, friend := range friends {
		item := friendItem(friend)
		// ponytail: one canonical lookup per row; store requester_id in the index if friend lists become large.
		friendship, err := s.store.GetFriend(ctx, friend.ID)
		if err != nil {
			s.cfg.logger.Error("failed to get friendship", "error", err, "friendship_id", friend.ID)
			return nil, status.Error(codes.Internal, "could not get friends")
		}
		item.UserId = friendship.UserID.String()
		item.FriendId = friendship.FriendID.String()
		items = append(items, item)
	}
	s.cfg.logger.Info("friends retrieved", "user_id", userID, "count", len(items))
	return &friendsv1.GetFriendsResponse{Items: items}, nil
}

func (s *FriendService) GetFriend(ctx context.Context, req *friendsv1.GetFriendRequest) (*friendsv1.GetFriendResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger := s.cfg.logger.With("user_id", userID, "friendship_id", req.FriendshipId)
	friendshipID, err := parseFriendID(req.FriendshipId, "friendship ID")
	if err != nil {
		return nil, err
	}
	friendship, err := s.store.GetFriend(ctx, friendshipID)
	if errors.Is(err, gocql.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "friendship not found")
	}
	if err != nil {
		logger.Error("failed to get friend", "error", err)
		return nil, status.Error(codes.Internal, "could not get friend")
	}
	if friendship.UserID != userID && friendship.FriendID != userID {
		return nil, status.Error(codes.PermissionDenied, "friendship does not belong to user")
	}
	logger.Info("friend retrieved", "friendship_id", friendship.ID)
	return &friendsv1.GetFriendResponse{Item: friendItem(friendship)}, nil
}

func (s *FriendService) RemoveFriend(ctx context.Context, req *friendsv1.RemoveFriendRequest) (*friendsv1.RemoveFriendResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger := s.cfg.logger.With("friendship_id", req.FriendshipId, "user_id", userID, "friend_id", req.FriendId)
	logger.Info("removing friend")
	friendship, err := s.friendship(ctx, req.FriendshipId, userID.String(), req.FriendId)
	if err != nil {
		logger.Error("failed to get friendship", "error", err)
		return nil, err
	}
	if friendship.Status == "pending" && friendship.UserID != userID {
		return nil, status.Error(codes.PermissionDenied, "only the requester can cancel a friend request")
	}
	if err := s.store.RemoveFriend(ctx, friendship.ID, friendship.UserID, friendship.FriendID); err != nil {
		logger.Error("failed to remove friend", "error", err)
		return nil, status.Error(codes.Internal, "could not remove friend")
	}
	logger.Info("friend removed")
	return &friendsv1.RemoveFriendResponse{Status: "removed"}, nil
}

func (s *FriendService) friendship(ctx context.Context, friendshipID, userID, friendID string) (db.Friend, error) {
	id, err := parseFriendID(friendshipID, "friendship ID")
	if err != nil {
		return db.Friend{}, err
	}
	user, friend, err := friendIDs(userID, friendID)
	if err != nil {
		return db.Friend{}, err
	}
	friendship, err := s.store.GetFriend(ctx, id)
	if errors.Is(err, gocql.ErrNotFound) {
		return db.Friend{}, status.Error(codes.NotFound, "friendship not found")
	}
	if err != nil {
		return db.Friend{}, status.Error(codes.Internal, "could not get friendship")
	}
	forward := friendship.UserID == user && friendship.FriendID == friend
	reverse := friendship.UserID == friend && friendship.FriendID == user
	if !forward && !reverse {
		return db.Friend{}, status.Error(codes.PermissionDenied, "friendship does not match the supplied users")
	}
	return friendship, nil
}

func friendIDs(userID, friendID string) (gocql.UUID, gocql.UUID, error) {
	user, err := parseFriendID(userID, "user ID")
	if err != nil {
		return gocql.UUID{}, gocql.UUID{}, err
	}
	friend, err := parseFriendID(friendID, "friend ID")
	if err != nil {
		return gocql.UUID{}, gocql.UUID{}, err
	}
	return user, friend, nil
}

func parseFriendID(value, name string) (gocql.UUID, error) {
	id, err := gocql.ParseUUID(value)
	if err != nil {
		return gocql.UUID{}, status.Error(codes.InvalidArgument, "valid "+name+" is required")
	}
	return id, nil
}

func userLookupError(err error) error {
	if errors.Is(err, gocql.ErrNotFound) {
		return status.Error(codes.NotFound, "user not found")
	}
	return status.Error(codes.Internal, "could not get user")
}

func friendItem(friend db.Friend) *friendsv1.GetFriendsItem {
	return &friendsv1.GetFriendsItem{UserId: friend.UserID.String(), FriendshipId: friend.ID.String(), FriendId: friend.FriendID.String(), FriendUsername: friend.FriendName, Status: friend.Status}
}
