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
	userID, err := serv.store.GetUserIDByUsername(ctx, username)
	if userID != (gocql.UUID{}) {
		return nil, status.Error(codes.AlreadyExists, "username already exists")
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
	serv.cfg.logger.Info("user created", "name", user.Name, "username", user.Username)
	return &usersV1.CreateUserResponse{UserId: user.ID.String(), Name: user.Name, Username: user.Username}, nil
}

/*
 * Since the only field that can be updated are the username,
 * and name we only need to handle that field in the update request.
 */
func (serv *UserService) UpdateUser(ctx context.Context, req *usersV1.UpdateUserRequest) (*usersV1.UpdateUserResponse, error) {
	username := strings.TrimSpace(req.Username)
	name := strings.TrimSpace(req.Name)
	if username == "" || name == "" {
		return nil, status.Error(codes.InvalidArgument, "username and name are required")
	}
	userID, err := gocql.ParseUUID(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user ID is required")
	}
	id, err := serv.store.GetUserIDByUsername(ctx, username)
	if id != (gocql.UUID{}) && id != userID {
		return nil, status.Error(codes.NotFound, "username already exists, please choose a different one")
	}
	user, err := serv.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	updatedUser, err := serv.store.UpdateUser(ctx, user, db.UpdateUserParams{Username: username, Name: name})
	if err != nil {
		serv.cfg.logger.Error("failed to update user", "error", err, "location", "UpdateUser")
		return nil, status.Error(codes.Internal, "failed to update user")
	}
	return &usersV1.UpdateUserResponse{
		UserId:   updatedUser.ID.String(),
		Username: updatedUser.Username,
		Name:     updatedUser.Name,
	}, nil
}

func (serv *UserService) GetUser(ctx context.Context, req *usersV1.GetUserRequest) (*usersV1.GetUserResponse, error) {
	userID, err := gocql.ParseUUID(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user ID is required")
	}
	user, err := serv.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "failed to get user")
	}
	return &usersV1.GetUserResponse{
		UserId:   user.ID.String(),
		Username: user.Username,
		Name:     user.Name,
	}, nil
}

func (serv *UserService) GetUserByUsername(ctx context.Context, req *usersV1.GetUserByUsernameRequest) (*usersV1.GetUserByUsernameResponse, error) {
	username := strings.TrimSpace(req.Username)
	userID, err := serv.store.GetUserIDByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "failed to get user")
	}

	user, err := serv.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get user")
	}
	return &usersV1.GetUserByUsernameResponse{
		UserId:   user.ID.String(),
		Name:     user.Name,
		Username: user.Username,
	}, nil
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
	userUUID, err := gocql.ParseUUID(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID")
	}
	if err := serv.store.DeleteUser(ctx, db.User{ID: userUUID}); err != nil {
		return nil, status.Error(codes.Internal, "could not delete user")
	}
	serv.cfg.logger.Info("Successfully deleted user", "user_id", userUUID)
	return &usersV1.DeleteUserResponse{Status: "user deleted successfully"}, nil
}
