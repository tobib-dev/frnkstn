package main

import (
	"context"
	"fmt"
	"log"

	pb "github.com/tobib-dev/frnkstn/api/proto/users/v1"
)

// Sample user server
type UserConfig struct {
	pb.UnimplementedUserServiceServer
	savedUsers []*pb.GetUserResponse
}

func (cfg *UserConfig) CreateUser(ctx context.Context, guest *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	users := cfg.savedUsers
	for _, us := range users {
		if guest.Username == us.Username {
			return &pb.CreateUserResponse{}, fmt.Errorf("Email: %s or git profile: %s already exists!!!", guest.Username)
		}
	}

	user := &pb.CreateUserResponse{
		Name:     guest.Name,
		Username: guest.Username,
	}
	su := &pb.GetUserResponse{
		Name:     user.Name,
		Username: user.Username,
	}
	users = append(users, su)

	log.Printf("Successfully created user: %s\n", user.Username)
	return user, nil
}
