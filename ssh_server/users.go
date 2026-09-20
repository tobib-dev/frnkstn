package main

import (
	"context"
	"fmt"
	"strconv"

	"charm.land/log/v2"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	github "github.com/tobib-dev/frnkstn/internal/github"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type userInfo struct {
	userID   string
	name     string
	username string
}

// getUser fetches the local account linked to a GitHub identity.
func getUser(githubID int64, grpcPort string) (userInfo, error) {
	log.Info("getUser", "github_id", githubID)

	conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return userInfo{}, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	response, err := usersV1.NewUserServiceClient(conn).GetUserByGHID(ctx, &usersV1.GetUserByGHIDRequest{GithubUserId: strconv.FormatInt(githubID, 10)})
	if err != nil {
		return userInfo{}, fmt.Errorf("get user: %w", err)
	}
	if response.UserId == "" {
		return userInfo{}, fmt.Errorf("get user returned no user ID")
	}
	return userInfo{userID: response.UserId, name: response.Name, username: response.Username}, nil
}

func createUser(username string, identity github.User, grpcPort string) (string, error) {
	log.Info("creating user", "github_user_id", identity)
	conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	response, err := usersV1.NewUserServiceClient(conn).CreateUser(ctx, &usersV1.CreateUserRequest{Username: username, Name: identity.Name, GithubUserId: strconv.FormatInt(identity.ID, 10)})
	if err != nil {
		return "", err
	}
	if response.UserId == "" {
		return "", fmt.Errorf("create user returned no user ID")
	}
	return response.UserId, nil
}

// getGitHubUser resolves the authenticated GitHub identity in the SSH server.
func getGitHubUser(accessToken string) (github.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()

	log.Info("getGitHubUser", "access_token", accessToken)
	return github.GetUser(ctx, accessToken)
}
