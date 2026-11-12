package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *serverService) InviteMember(
	ctx context.Context, req *serversv1.InviteMemberRequest,
) (response *serversv1.InviteMemberResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "InviteMember", "server_id", req.GetServerId(),
		"member_id", req.GetMemberId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := parseID(req.GetServerId(), "server ID")
	if err != nil {
		return nil, err
	}
	receiverID, err := parseID(req.GetMemberId(), "member ID")
	if err != nil {
		return nil, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, s.memberError("get server", err)
	}
	if userID != server.Owner {
		moderator, err := s.members.IsServerModerator(ctx, serverID, userID)
		if err != nil {
			return nil, s.memberError("check moderator", err)
		}
		if !moderator {
			if _, err := s.members.GetMember(ctx, serverID, userID); err != nil {
				if errors.Is(err, gocql.ErrNotFound) {
					return nil, status.Error(
						codes.PermissionDenied, "server membership required",
					)
				}
				return nil, s.memberError("check membership", err)
			}
		}
	}
	if _, err := s.users.GetUserByID(ctx, receiverID); err != nil {
		logger.Error("failed to get invitee", "error", err)
		return nil, userLookupError(err)
	}
	moderator, err := s.members.IsServerModerator(ctx, serverID, receiverID)
	if err != nil {
		return nil, s.memberError("check invitee moderator", err)
	}
	if receiverID == server.Owner || moderator {
		return nil, s.memberError("invite member", db.ErrAlreadyServerMember)
	}
	_, err = s.members.GetMember(ctx, serverID, receiverID)
	if err == nil {
		return nil, s.memberError("invite member", db.ErrAlreadyServerMember)
	}
	if !errors.Is(err, gocql.ErrNotFound) {
		return nil, s.memberError("check invitee membership", err)
	}
	invite := db.ShareCode{
		Code: rand.Text(), Sender: userID,
		Receiver: receiverID, ServerID: serverID,
	}
	if err := s.members.ShareServer(ctx, invite); err != nil {
		return nil, s.memberError("invite member", err)
	}
	s.notifyServerInvite(invite, server.ServerName)
	return &serversv1.InviteMemberResponse{}, nil
}

func (s *serverService) JoinServer(
	ctx context.Context, req *serversv1.JoinServerRequest,
) (response *serversv1.JoinServerResponse, rpcErr error) {
	logger := s.cfg.logger.With("operation", "JoinServer")
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	code := strings.TrimSpace(req.GetShareCode())
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "share code is required")
	}
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		logger.Error("failed to get invitation recipient", "error", err)
		return nil, userLookupError(err)
	}
	server, err := s.members.JoinServer(ctx, db.JoinServerArgs{
		UserID: userID, UserName: user.Username, Code: code,
	})
	if err != nil {
		return nil, s.memberError("join server", err)
	}
	return &serversv1.JoinServerResponse{Item: serverItem(server)}, nil
}

func (s *serverService) GetServerByCode(
	ctx context.Context, req *serversv1.GetServerByCodeRequest,
) (response *serversv1.GetServerByCodeResponse, rpcErr error) {
	logger := s.cfg.logger.With("operation", "GetServerByCode")
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	code := strings.TrimSpace(req.GetShareCode())
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "share code is required")
	}
	server, err := s.members.GetServerByCode(ctx, code, userID)
	if err != nil {
		return nil, s.memberError("get server by code", err)
	}
	return &serversv1.GetServerByCodeResponse{Item: serverItem(server)}, nil
}

func (s *serverService) GetMembers(
	ctx context.Context, req *serversv1.GetMembersRequest,
) (response *serversv1.GetMembersResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "GetMembers", "server_id", req.GetServerId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := parseID(req.GetServerId(), "server ID")
	if err != nil {
		return nil, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, s.memberError("get server", err)
	}
	moderator := userID == server.Owner
	if !moderator {
		moderator, err = s.members.IsServerModerator(ctx, serverID, userID)
		if err != nil {
			return nil, s.memberError("check moderator", err)
		}
	}
	if !moderator {
		_, err = s.members.GetMember(ctx, serverID, userID)
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.PermissionDenied, "server membership required")
		}
		if err != nil {
			return nil, s.memberError("check membership", err)
		}
	}
	members, err := s.members.GetMembers(ctx, serverID)
	if err != nil {
		return nil, s.memberError("get members", err)
	}
	items := make([]*serversv1.MemberItem, len(members))
	for i, member := range members {
		items[i] = &serversv1.MemberItem{
			ServerId:   member.ServerID.String(),
			MemberId:   member.MemberID.String(),
			MemberName: member.MemberName,
		}
	}
	return &serversv1.GetMembersResponse{Items: items}, nil
}

func (s *serverService) LeaveServer(
	ctx context.Context, req *serversv1.LeaveServerRequest,
) (response *serversv1.LeaveServerResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "LeaveServer", "server_id", req.GetServerId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := parseID(req.GetServerId(), "server ID")
	if err != nil {
		return nil, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, s.memberError("get server", err)
	}
	if userID == server.Owner {
		return nil, status.Error(
			codes.FailedPrecondition, "transfer ownership before leaving",
		)
	}
	if _, err := s.members.GetMember(ctx, serverID, userID); err != nil {
		return nil, s.memberError("get member", err)
	}
	removedAt := time.Now().UTC()
	if err := s.members.LeaveServer(ctx, userID, serverID); err != nil {
		return nil, s.memberError("leave server", err)
	}
	s.queueServerCleanup(serverCleanupEvent{
		Operation: cleanupMemberRemoved, ServerID: serverID.String(),
		UserID: userID.String(), RemovedAt: removedAt,
	})
	return &serversv1.LeaveServerResponse{}, nil
}

func (s *serverService) DeleteMember(
	ctx context.Context, req *serversv1.DeleteMemberRequest,
) (response *serversv1.DeleteMemberResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "DeleteMember", "server_id", req.GetServerId(),
		"member_id", req.GetMemberId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := parseID(req.GetServerId(), "server ID")
	if err != nil {
		return nil, err
	}
	memberID, err := parseID(req.GetMemberId(), "member ID")
	if err != nil {
		return nil, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, s.memberError("get server", err)
	}
	if userID != server.Owner {
		moderator, err := s.members.IsServerModerator(ctx, serverID, userID)
		if err != nil {
			return nil, s.memberError("check moderator", err)
		}
		if !moderator {
			return nil, status.Error(codes.PermissionDenied, "moderator permission required")
		}
	}
	if memberID == server.Owner {
		return nil, status.Error(codes.PermissionDenied, "cannot remove server owner")
	}
	member, err := s.members.GetMember(ctx, serverID, memberID)
	if err != nil {
		return nil, s.memberError("get member", err)
	}
	removedAt := time.Now().UTC()
	if err := s.members.LeaveServer(ctx, member.MemberID, serverID); err != nil {
		return nil, s.memberError("remove member", err)
	}
	s.queueServerCleanup(serverCleanupEvent{
		Operation: cleanupMemberRemoved, ServerID: serverID.String(),
		UserID: member.MemberID.String(), RemovedAt: removedAt,
	})
	return &serversv1.DeleteMemberResponse{}, nil
}

func logMemberRPCError(logger *slog.Logger, err error) {
	if err == nil {
		return
	}
	fields := []any{"error", err, "code", status.Code(err).String()}
	if status.Code(err) == codes.Internal {
		logger.Error("server member request failed", fields...)
		return
	}
	logger.Warn("server member request rejected", fields...)
}

func (s *serverService) memberError(operation string, err error) error {
	switch {
	case errors.Is(err, gocql.ErrNotFound):
		return status.Error(codes.NotFound, "server, member or invitation not found")
	case errors.Is(err, db.ErrAlreadyServerMember):
		return status.Error(codes.AlreadyExists, "user already belongs to server")
	case errors.Is(err, db.ErrInvitationRecipient):
		return status.Error(codes.PermissionDenied, "invitation belongs to another user")
	case errors.Is(err, db.ErrInvitationUsed):
		return status.Error(codes.FailedPrecondition, "invitation has been used")
	case errors.Is(err, db.ErrShareCodeExists):
		return status.Error(codes.AlreadyExists, "invitation code already exists")
	default:
		s.cfg.logger.Error("server member operation failed",
			"operation", operation, "error", err)
		return status.Error(codes.Internal, "could not "+operation)
	}
}
