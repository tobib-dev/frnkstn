package main

import (
	"context"
	"fmt"
	"strconv"

	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type sessionInfo struct {
	sessionID string
	userID    string
}

func createSession(token tokenInfo, grpcPort string) (sessionInfo, error) {
	grpcHost := "localhost:" + grpcPort
	conn, err := grpc.NewClient(grpcHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to connect to grpc server: %w", err)
	}
	defer conn.Close()

	client := sessionsV1.NewSessionServiceClient(conn)
	resp, err := client.CreateSession(context.Background(), &sessionsV1.CreateSessionRequest{
		AccessToken:           token.AccessToken,
		AccessTokenExpiresAt:  strconv.Itoa(token.ExpiresIn),
		RefreshToken:          token.RefreshToken,
		RefreshTokenExpiresAt: strconv.Itoa(token.RefreshTokenExpiresIn),
	})
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to create session: %w", err)
	}
	sessInfo := sessionInfo{
		sessionID: resp.SessionId,
		userID:    resp.UserId,
	}

	return sessInfo, nil
}

func getSession(token tokenInfo, grpcPort string) (sessionInfo, error) {
	grpcHost := "localhost:" + grpcPort
	conn, err := grpc.NewClient(grpcHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to connect to grpc server: %w", err)
	}
	defer conn.Close()

	client := sessionsV1.NewSessionServiceClient(conn)
	resp, err := client.GetSession(context.Background(), &sessionsV1.GetSessionRequest{
		AccessToken:           token.AccessToken,
		AccessTokenExpiresAt:  strconv.Itoa(token.ExpiresIn),
		RefreshToken:          token.RefreshToken,
		RefreshTokenExpiresAt: strconv.Itoa(token.RefreshTokenExpiresIn),
	})
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to create session: %w", err)
	}
	sessInfo := sessionInfo{
		sessionID: resp.SessionId,
		userID:    resp.UserId,
	}

	return sessInfo, nil
}
