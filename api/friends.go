package main

import (
	"context"
	"errors"

	"github.com/gocql/gocql"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FriendService struct {
	cfg   *Config
	store db.FriendStore
	users db.UserStore
	friendsv1.UnimplementedFriendServiceServer
}

func NewFriendService(cfg *Config) *FriendService {
	return &FriendService{cfg: cfg, store: &cfg.db.session, users: &cfg.db.session}
}

func (s *FriendService) AddFriend(ctx context.Context, req *friendsv1.AddFriendRequest) (*friendsv1.AddFriendResponse, error) {
	logger := s.cfg.logger.With("user_id", req.UserId, "friend_id", req.FriendId)
	logger.Info("adding friend")

	userID, friendID, err := friendIDs(req.UserId, req.FriendId)
	if err != nil {
		logger.Warn("invalid add friend request", "error", err)
		return nil, err
	}
	if userID == friendID {
		logger.Warn("cannot add user as their own friend")
		return nil, status.Error(codes.InvalidArgument, "cannot add yourself as a friend")
	}
	if _, err := s.users.GetUserByID(ctx, userID); err != nil {
		logger.Error("failed to get requesting user", "error", err)
		return nil, userLookupError(err)
	}
	friendUser, err := s.users.GetUserByID(ctx, friendID)
	if err != nil {
		logger.Error("failed to get requested friend", "error", err)
		return nil, userLookupError(err)
	}

	friendship := db.Friend{ID: gocql.TimeUUID(), UserID: userID, FriendID: friendID, FriendName: friendUser.Username}
	friendship, err = s.store.AddFriend(ctx, friendship)
	if err != nil {
		logger.Error("failed to add friend", "error", err, "friendship_id", friendship.ID)
		return nil, status.Error(codes.Internal, "could not add friend")
	}
	logger.Info("friend added", "friendship_id", friendship.ID, "status", friendship.Status)
	return &friendsv1.AddFriendResponse{FriendshipId: friendship.ID.String(), FriendId: friendship.FriendID.String(), Status: friendship.Status}, nil
}

func (s *FriendService) AcceptFriend(ctx context.Context, req *friendsv1.AcceptFriendRequest) (*friendsv1.AcceptFriendResponse, error) {
	logger := s.cfg.logger.With("friendship_id", req.FriendshipId, "user_id", req.UserId, "friend_id", req.FriendId)
	logger.Info("accepting friend")
	friendship, err := s.friendship(ctx, req.FriendshipId, req.UserId, req.FriendId)
	if err != nil {
		logger.Error("failed to get friendship", "error", err)
		return nil, err
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
	logger := s.cfg.logger.With("friendship_id", req.FriendshipId, "user_id", req.UserId, "friend_id", req.FriendId)
	logger.Info("rejecting friend")
	friendship, err := s.friendship(ctx, req.FriendshipId, req.UserId, req.FriendId)
	if err != nil {
		logger.Error("failed to get friendship", "error", err)
		return nil, err
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
	userID, err := parseFriendID(req.UserId, "user ID")
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
		items = append(items, friendItem(friend))
	}
	s.cfg.logger.Info("friends retrieved", "user_id", userID, "count", len(items))
	return &friendsv1.GetFriendsResponse{Items: items}, nil
}

func (s *FriendService) GetFriend(ctx context.Context, req *friendsv1.GetFriendRequest) (*friendsv1.GetFriendResponse, error) {
	logger := s.cfg.logger.With("user_id", req.UserId, "friend_id", req.FriendId)
	friendship, err := s.findFriendship(ctx, req.UserId, req.FriendId)
	if err != nil {
		logger.Error("failed to get friend", "error", err)
		return nil, err
	}
	logger.Info("friend retrieved", "friendship_id", friendship.ID)
	return &friendsv1.GetFriendResponse{Item: friendItem(friendship)}, nil
}

func (s *FriendService) RemoveFriend(ctx context.Context, req *friendsv1.RemoveFriendRequest) (*friendsv1.RemoveFriendResponse, error) {
	logger := s.cfg.logger.With("friendship_id", req.FriendshipId, "user_id", req.UserId, "friend_id", req.FriendId)
	logger.Info("removing friend")
	friendship, err := s.friendship(ctx, req.FriendshipId, req.UserId, req.FriendId)
	if err != nil {
		logger.Error("failed to get friendship", "error", err)
		return nil, err
	}
	if err := s.store.RemoveFriend(ctx, friendship.ID, friendship.UserID); err != nil {
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
	if friendship.UserID != user || friendship.FriendID != friend {
		return db.Friend{}, status.Error(codes.PermissionDenied, "friendship does not match the supplied users")
	}
	return friendship, nil
}

func (s *FriendService) findFriendship(ctx context.Context, userID, friendID string) (db.Friend, error) {
	user, friend, err := friendIDs(userID, friendID)
	if err != nil {
		return db.Friend{}, err
	}
	friends, err := s.store.GetFriends(ctx, user)
	if err != nil {
		return db.Friend{}, status.Error(codes.Internal, "could not get friends")
	}
	for _, friendship := range friends {
		if friendship.FriendID == friend {
			return friendship, nil
		}
	}
	return db.Friend{}, status.Error(codes.NotFound, "friendship not found")
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
	return &friendsv1.GetFriendsItem{UserId: friend.UserID.String(), FriendshipId: friend.ID.String(), FriendId: friend.FriendID.String(), FriendUsername: friend.FriendName}
}
