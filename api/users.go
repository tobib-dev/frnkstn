package main

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gocql/gocql"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserService struct {
	cfg   *Config
	store db.UserStore
	usersV1.UnimplementedUserServiceServer
}

func NewUserService(cfg *Config) *UserService {
	return &UserService{cfg: cfg, store: &cfg.db.session}
}

func (serv *UserService) CreateUser(ctx context.Context, guest *usersV1.CreateUserRequest) (*usersV1.CreateUserResponse, error) {
	username := strings.TrimSpace(guest.Username)
	userID := gocql.TimeUUID()

	if username == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}
	githubID, err := strconv.ParseInt(guest.GithubUserId, 10, 64)
	if err != nil || githubID <= 0 {
		return nil, status.Error(codes.InvalidArgument, "GitHub ID is required")
	}
	user, err := serv.store.CreateUser(ctx, db.User{
		ID:       userID,
		GitHubID: githubID,
		Name:     guest.Name,
		Username: username,
	})
	if err != nil {
		serv.cfg.logger.Error("failed to create user", "error", err)
		return nil, status.Error(codes.Internal, "could not create user")
	}
	serv.cfg.logger.Info("user created", "user_id", user.ID.String(), "name", user.Name, "username", user.Username)
	return &usersV1.CreateUserResponse{UserId: user.ID.String(), Name: user.Name, Username: user.Username}, nil
}

func (serv *UserService) UpdateUser(ctx context.Context, req *usersV1.UpdateUserRequest) (*usersV1.UpdateUserResponse, error) {
	//
	return &usersV1.UpdateUserResponse{}, nil
}

func (serv *UserService) GetUser(ctx context.Context, req *usersV1.GetUserRequest) (*usersV1.GetUserResponse, error) {
	return &usersV1.GetUserResponse{}, nil
}

func (serv *UserService) GetUserByGHID(ctx context.Context, req *usersV1.GetUserByGHIDRequest) (*usersV1.GetUserByGHIDResponse, error) {
	githubID, err := strconv.ParseInt(req.GithubUserId, 10, 64)
	if err != nil || githubID <= 0 {
		return nil, status.Error(codes.InvalidArgument, "GitHub ID is required")
	}
	user, err := serv.store.GetUserByGitHubID(ctx, githubID)
	if err != nil {
		serv.cfg.logger.Error("error getting user by GitHub ID", "error", err)
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "could not get user")
	}
	return &usersV1.GetUserByGHIDResponse{GithubUserId: req.GithubUserId, UserId: user.ID.String(), Name: user.Name, Username: user.Username}, nil
}

func (serv *UserService) DeleteUser(ctx context.Context, req *usersV1.DeleteUserRequest) (*usersV1.DeleteUserResponse, error) {
	return &usersV1.DeleteUserResponse{}, nil
}
