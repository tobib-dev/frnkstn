package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gocql/gocql"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"github.com/tobib-dev/frnkstn/api/auth"
	"github.com/tobib-dev/frnkstn/api/db"
	githubauth "github.com/tobib-dev/frnkstn/internal/github"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	internalAccessTokenLifetime  = 15 * time.Minute
	internalRefreshTokenLifetime = 7 * 24 * time.Hour
)

type SessionService struct {
	cfg               *Config
	store             db.SessionStore
	tokens            db.InternalRefreshTokenStore
	users             db.UserStore
	verifyGitHubToken func(context.Context, string) (githubauth.User, error)
	sessionsV1.UnimplementedSessionServiceServer
}

func NewSessionService(cfg *Config) *SessionService {
	return &SessionService{
		cfg: cfg, store: &cfg.db.session, tokens: &cfg.db.session, users: &cfg.db.session,
		verifyGitHubToken: githubauth.GetUser,
	}
}

func (serv *SessionService) CreateSession(ctx context.Context, sessionInfo *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	sessionID := gocql.UUIDFromTime(time.Now())
	serv.cfg.logger.Info("creating session")

	if sessionInfo.GithubAccessToken == "" {
		return nil, status.Error(codes.InvalidArgument, "GitHub access token is required")
	}
	identity, err := serv.verifyGitHubToken(ctx, sessionInfo.GithubAccessToken)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid GitHub access token")
	}
	user, err := serv.users.GetUserByGitHubID(ctx, identity.ID)
	if errors.Is(err, gocql.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "could not get user")
	}
	userID := user.ID
	now := time.Now()
	githubAccessTokenExpiresAt, err := tokenExpiry(now, sessionInfo.GithubAccessTokenExpiresAt)
	if err != nil {
		return nil, err
	}
	githubRefreshTokenExpiresAt, err := tokenExpiry(now, sessionInfo.GithubRefreshTokenExpiresAt)
	if err != nil {
		return nil, err
	}
	sess, err := serv.store.CreateSession(ctx, db.CreateSessionParams{
		ID: sessionID, UserID: userID,
		GitHubAccessToken: sessionInfo.GithubAccessToken, GitHubAccessTokenExpiresAt: githubAccessTokenExpiresAt,
		GitHubRefreshToken: sessionInfo.GithubRefreshToken, GitHubRefreshTokenExpiresAt: githubRefreshTokenExpiresAt,
	})
	if err != nil {
		serv.cfg.logger.Info("failed to create session", "error", err)
		return nil, status.Error(codes.Internal, "could not create session")
	}

	internalRefreshToken, err := serv.internalRefreshToken(ctx, identity.ID, userID, now)
	if err != nil {
		serv.cfg.logger.Error("failed to create internal refresh token", "error", err, "user_id", userID)
		return nil, status.Error(codes.Internal, "could not create refresh token")
	}
	internalAccessToken, err := auth.MakeJWT(userID.String(), auth.DefaultUserPermissions(), serv.cfg.signingKey, internalAccessTokenLifetime)
	if err != nil {
		serv.cfg.logger.Error("failed to create internal access token", "error", err, "user_id", userID)
		return nil, status.Error(codes.Internal, "could not create access token")
	}

	serv.cfg.logger.Info("session successfully created", "session_id", sess.ID.String())
	return &sessionsV1.CreateSessionResponse{
		SessionId: sess.ID.String(), UserId: sess.UserID.String(), InternalAccessToken: internalAccessToken,
		InternalRefreshToken: internalRefreshToken.Token, InternalAccessTokenExpiresAt: now.Add(internalAccessTokenLifetime).Format(time.RFC3339Nano),
	}, nil
}

func (serv *SessionService) internalRefreshToken(ctx context.Context, githubID int64, userID gocql.UUID, now time.Time) (db.InternalRefreshToken, error) {
	token, err := serv.tokens.GetInternalRefreshTokenByGitHubID(ctx, githubID)
	if err == nil && token.UserID == userID && token.RevokedAt.IsZero() && token.Expiration.After(now) {
		return token, nil
	}
	if err != nil && !errors.Is(err, gocql.ErrNotFound) {
		return db.InternalRefreshToken{}, err
	}
	value, err := auth.MakeRefreshToken()
	if err != nil {
		return db.InternalRefreshToken{}, err
	}
	token = db.InternalRefreshToken{Token: value, UserID: userID, GitHubID: githubID, Expiration: now.Add(internalRefreshTokenLifetime)}
	if err := serv.tokens.CreateInternalRefreshToken(ctx, token); err != nil {
		return db.InternalRefreshToken{}, err
	}
	return token, nil
}

func (serv *SessionService) GetSession(ctx context.Context, req *sessionsV1.GetSessionRequest) (*sessionsV1.GetSessionResponse, error) {
	serv.cfg.logger.Info("get session", "session_id", req.SessionId)
	userID, err := authenticatedUserID(ctx, serv.cfg, "")
	if err != nil {
		return nil, err
	}

	sess, err := serv.store.GetSession(ctx, db.GetSessionParams{
		ID: gocql.ParseUUIDMust(req.SessionId),
	})
	if err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "session not found")
		}
		return nil, status.Error(codes.Internal, "could not get session")
	}
	if sess.UserID != userID {
		return nil, status.Error(codes.PermissionDenied, "session does not belong to user")
	}

	resp := &sessionsV1.GetSessionResponse{
		SessionId:                  sess.ID.String(),
		UserId:                     sess.UserID.String(),
		GithubAccessTokenExpiresAt: sess.GitHubAccessTokenExpiresAt.String(),
		GithubRefreshToken:         sess.GitHubRefreshToken,
		SignedOutAt:                sess.SignedOutAt.String(),
	}
	serv.cfg.logger.Info("successfully retrieved session", "session_id", req.SessionId)
	return resp, nil
}

func (serv *SessionService) GetSessionByUser(ctx context.Context, req *sessionsV1.GetSessionByUserRequest) (*sessionsV1.GetSessionByUserResponse, error) {
	serv.cfg.logger.Info("get session by user")
	userID, err := authenticatedUserID(ctx, serv.cfg, "")
	if err != nil {
		return nil, err
	}

	sess, err := serv.store.GetSessionByUser(ctx, db.GetSessionByUserParams{
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "session not found")
		}
		return nil, status.Error(codes.Internal, "could not get session")
	}

	resp := &sessionsV1.GetSessionByUserResponse{
		SessionId:                   sess.SessionID.String(),
		UserId:                      sess.UserID.String(),
		GithubRefreshTokenExpiresAt: sess.GitHubRefreshTokenExpiresAt.String(),
		GithubRefreshToken:          sess.GitHubRefreshToken,
		SignedOutAt:                 sess.SignedOutAt.String(),
	}
	serv.cfg.logger.Info("successfully retrieved session by user", "session_id", sess.SessionID.String())
	return resp, nil
}

func (serv *SessionService) UpdateSession(ctx context.Context, req *sessionsV1.UpdateSessionRequest) (*sessionsV1.UpdateSessionResponse, error) {
	serv.cfg.logger.Info("update session", "session_id", req.SessionId)
	userID, err := authenticatedUserID(ctx, serv.cfg, "")
	if err != nil {
		return nil, err
	}

	sess, err := serv.store.UpdateSession(ctx, db.UpdateSessionParams{
		ID:          gocql.ParseUUIDMust(req.SessionId),
		UserID:      userID,
		SignedOutAt: time.Now(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "could not update session")
	}
	resp := &sessionsV1.UpdateSessionResponse{
		SessionId:                  sess.ID.String(),
		UserId:                     sess.UserID.String(),
		GithubAccessTokenExpiresAt: sess.GitHubAccessTokenExpiresAt.String(),
		GithubRefreshToken:         sess.GitHubRefreshToken,
		SignedOutAt:                sess.SignedOutAt.Format(time.RFC3339Nano),
	}
	serv.cfg.logger.Info("successfully updated session", "session_id", sess.ID.String())
	return resp, nil
}

// The current wire contract sends token lifetimes as decimal seconds.
func tokenExpiry(now time.Time, seconds string) (time.Time, error) {
	n, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil || n < 0 || n > int64((1<<63-1)/time.Second) {
		return time.Time{}, status.Error(codes.InvalidArgument, "invalid token lifetime")
	}
	if n == 0 {
		return time.Time{}, nil
	}
	return now.Add(time.Duration(n) * time.Second), nil
}
