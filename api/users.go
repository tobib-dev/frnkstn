package main

import (
	"context"
	//"fmt"

	"github.com/google/uuid"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	usersv1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	//"github.com/tobib-dev/frnkstn/api/db"
)

// Sample user server
type UserService struct {
	cfg *Config
	usersV1.UnimplementedUserServiceServer
}

func NewUserService(cfg *Config) *UserService {
	return &UserService{cfg: cfg}
}

func (serv *UserService) CreateUser(ctx context.Context, guest *usersV1.CreateUserRequest) (*usersV1.CreateUserResponse, error) {
	userId := uuid.New()
	serv.cfg.logger.Info("creating user", "userID", userId.String(), "username", guest.Username, "service", "user")
	return &usersv1.CreateUserResponse{
		UserId: userId.String(),
	}, nil
}

func (serv *UserService) UpdateUser(ctx context.Context, req *usersV1.UpdateUserRequest) (*usersV1.UpdateUserResponse, error) {
	//
	return &usersV1.UpdateUserResponse{}, nil
}

func (serv *UserService) GetUser(ctx context.Context, req *usersV1.GetUserRequest) (*usersV1.GetUserResponse, error) {
	return &usersv1.GetUserResponse{}, nil
}

func (serv *UserService) DeleteUser(ctx context.Context, req *usersV1.DeleteUserRequest) (*usersV1.DeleteUserResponse, error) {
	return &usersv1.DeleteUserResponse{}, nil
}
