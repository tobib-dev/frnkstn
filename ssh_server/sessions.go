package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
)

type sessionInfo struct {
	sessionID, userID, internalAccessToken, internalRefreshToken string
	signedOutAt                                                  time.Time
}

func createSession(token githubTokenInfo, userID, grpcPort string) (sessionInfo, error) {
	if userID == "" {
		return sessionInfo{}, fmt.Errorf("cannot create session without user ID")
	}
	grpcHost := "localhost:" + grpcPort
	conn, err := newGRPCClient(grpcHost)
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to connect to grpc server: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	client := sessionsV1.NewSessionServiceClient(conn)
	resp, err := client.CreateSession(ctx, &sessionsV1.CreateSessionRequest{
		GithubAccessToken:           token.AccessToken,
		GithubAccessTokenExpiresAt:  strconv.Itoa(token.ExpiresIn),
		GithubRefreshToken:          token.RefreshToken,
		GithubRefreshTokenExpiresAt: strconv.Itoa(token.RefreshTokenExpiresIn),
	})
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to create session: %w", err)
	}
	if resp.SessionId == "" || resp.UserId != userID || resp.InternalAccessToken == "" || resp.InternalRefreshToken == "" {
		return sessionInfo{}, fmt.Errorf("invalid session response")
	}
	sessInfo := sessionInfo{
		sessionID:            resp.SessionId,
		userID:               resp.UserId,
		internalAccessToken:  resp.InternalAccessToken,
		internalRefreshToken: resp.InternalRefreshToken,
	}

	return sessInfo, nil
}

func getSession(sessionID, internalAccessToken, grpcPort string) (sessionInfo, error) {
	grpcHost := "localhost:" + grpcPort
	conn, err := newGRPCClient(grpcHost)
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to connect to grpc server: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	ctx = bearerContext(ctx, internalAccessToken)
	client := sessionsV1.NewSessionServiceClient(conn)
	resp, err := client.GetSession(ctx, &sessionsV1.GetSessionRequest{
		SessionId: sessionID,
	})
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to get session: %w", err)
	}

	signedOutAt, err := time.Parse(time.RFC3339, resp.SignedOutAt)
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to parse signed out at: %w", err)
	}

	sessInfo := sessionInfo{
		sessionID:   resp.SessionId,
		userID:      resp.UserId,
		signedOutAt: signedOutAt,
	}

	return sessInfo, nil
}

func updateSession(sessionID, internalAccessToken, grpcPort string) (sessionInfo, error) {
	if sessionID == "" || internalAccessToken == "" {
		return sessionInfo{}, fmt.Errorf("cannot sign out without session ID and access token")
	}
	grpcHost := "localhost:" + grpcPort
	conn, err := newGRPCClient(grpcHost)
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to connect to grpc server: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	ctx = bearerContext(ctx, internalAccessToken)
	client := sessionsV1.NewSessionServiceClient(conn)
	resp, err := client.UpdateSession(ctx, &sessionsV1.UpdateSessionRequest{
		SessionId: sessionID,
	})
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to update session: %w", err)
	}

	signedOutAt, err := time.Parse(time.RFC3339, resp.SignedOutAt)
	if err != nil {
		return sessionInfo{}, fmt.Errorf("failed to parse signed out at: %w", err)
	}

	sessInfo := sessionInfo{
		sessionID:   resp.SessionId,
		userID:      resp.UserId,
		signedOutAt: signedOutAt,
	}

	return sessInfo, nil
}
