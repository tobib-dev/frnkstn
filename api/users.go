package main

import (
	"context"
	"fmt"
	"log"

	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	usersv1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"github.com/tobib-dev/frnkstn/api/db"
)

// Sample user server
type UserService struct {
	db *db.DB
	usersV1.UnimplementedUserServiceServer
}

func NewUserService(db *db.DB) *UserService {
	return &UserService{db: db}
}

func (cfg *UserService) CreateUser(ctx context.Context, guest *usersV1.CreateUserRequest) (*usersV1.CreateUserResponse, error) {
	var users []string
	for _, us := range users {
		if guest.Username == us.Username {
			return &usersV1.CreateUserResponse{}, fmt.Errorf("User - %s already exists!!!", guest.Username)
		}
	}

	user := &usersV1.CreateUserResponse{
		Name:     guest.Name,
		Username: guest.Username,
	}
	su := &usersV1.GetUserResponse{
		Name:     user.Name,
		Username: user.Username,
	}
	users = append(users, su)

	log.Printf("Successfully created user: %s\n", user.Username)
	return user, nil
}

func (cfg *UserService) UpdateUser(ctx context.Context, req *usersV1.UpdateUserRequest) (*usersV1.UpdateUserResponse, error) {
	//
	return &usersV1.UpdateUserResponse{}, nil
}

func (cfg *UserService) GetUser(ctx context.Context, req *usersV1.GetUserRequest) (*usersV1.GetUserResponse, error) {
	return &usersv1.GetUserResponse{}, nil
}

func (cfg *UserService) DeleteUser(ctx context.Context, req *usersV1.DeleteUserRequest) (*usersV1.DeleteUserResponse, error) {
	return &usersv1.DeleteUserResponse{}, nil
}
