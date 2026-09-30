package main

import (
	"context"

	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	"github.com/tobib-dev/frnkstn/api/db"
)

type FriendService struct {
	cfg   *Config
	store db.FriendStore
	friendsv1.UnimplementedFriendServiceServer
}

func NewFriendService(cfg *Config) *FriendService {
	return &FriendService{
		cfg:   cfg,
		store: &cfg.db.session,
	}
}

type FriendStatus int

const (
	StatusPending FriendStatus = iota
	StatusAccepted
	StatusRejected
	StatusBlocked
	StatusRemoved
)

/*
 * Send a friend request, return status pending
 */
func (s *FriendService) AddFriend(ctx context.Context, req *friendsv1.AddFriendRequest) (*friendsv1.AddFriendResponse, error) {
	return &friendsv1.AddFriendResponse{}, nil
}

/*
 * Accept a friend request, return status accepted
 */
func (s *FriendService) AcceptFriend(ctx context.Context, req *friendsv1.AcceptFriendRequest) (*friendsv1.AcceptFriendResponse, error) {
	return &friendsv1.AcceptFriendResponse{}, nil
}

/*
 * Reject a friend request, return status rejected
 */
func (s *FriendService) RejectFriend(ctx context.Context, req *friendsv1.RejectFriendRequest) (*friendsv1.RejectFriendResponse, error) {
	return &friendsv1.RejectFriendResponse{}, nil
}

/*
 * Get a list of friends
 */
func (s *FriendService) GetFriends(ctx context.Context, req *friendsv1.GetFriendsRequest) (*friendsv1.GetFriendsResponse, error) {
	return &friendsv1.GetFriendsResponse{}, nil
}

/*
 * Retrieve a single friend
 */
func (s *FriendService) GetFriend(ctx context.Context, req *friendsv1.GetFriendRequest) (*friendsv1.GetFriendResponse, error) {
	return &friendsv1.GetFriendResponse{}, nil
}

/*
 * Remove a friend, return status removed
 */
func (s *FriendService) RemoveFriend(ctx context.Context, req *friendsv1.RemoveFriendRequest) (*friendsv1.RemoveFriendResponse, error) {
	return &friendsv1.RemoveFriendResponse{}, nil
}
