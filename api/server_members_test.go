package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testServerMemberStore struct {
	db.ServerMemberStore
	member      db.Member
	moderator   bool
	moderatorID gocql.UUID
	joinArgs    db.JoinServerArgs
	joinErr     error
	leaveUser   gocql.UUID
	leaveServer gocql.UUID
	revoked     bool
	revokeErr   error
	lists       int
	invites     []db.ShareCode
}

func (s *testServerMemberStore) ShareServer(
	_ context.Context, invite db.ShareCode,
) error {
	s.invites = append(s.invites, invite)
	return nil
}

func TestInviteMemberPermissionsAndGeneratedCode(t *testing.T) {
	owner, member, receiver := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	store := &testServerMemberStore{member: db.Member{
		ServerID: gocql.TimeUUID(), MemberID: member,
	}}
	service := memberTestService(store, owner)
	service.users = &testUserStore{users: map[gocql.UUID]db.User{
		receiver: {ID: receiver}, member: {ID: member},
	}}
	req := &serversv1.InviteMemberRequest{
		ServerId: store.member.ServerID.String(), MemberId: receiver.String(),
	}
	_, err := service.InviteMember(
		authenticatedTestContext(gocql.TimeUUID()), req,
	)
	if status.Code(err) != codes.PermissionDenied || len(store.invites) != 0 {
		t.Fatalf("outsider could invite: %v", err)
	}
	for _, sender := range []gocql.UUID{owner, member} {
		service.n10n = &notificationService{
			cfg: service.cfg, store: &testNotificationStore{}, broker: &testNotificationBroker{},
		}
		response, err := service.InviteMember(authenticatedTestContext(sender), req)
		if err != nil || response == nil {
			t.Fatalf("invite failed: %v", err)
		}
		invite := store.invites[len(store.invites)-1]
		if invite.Sender != sender || invite.Receiver != receiver ||
			invite.ServerID != store.member.ServerID || len(invite.Code) < 26 {
			t.Fatal("incorrect invitation identity or missing random code")
		}
	}
	if store.invites[0].Code == store.invites[1].Code {
		t.Fatal("invitations reused a code")
	}
	req.MemberId = member.String()
	_, err = service.InviteMember(authenticatedTestContext(owner), req)
	if status.Code(err) != codes.AlreadyExists || len(store.invites) != 2 {
		t.Fatalf("existing member was invited: %v", err)
	}
}

func (s *testServerMemberStore) GetMember(
	_ context.Context, serverID, memberID gocql.UUID,
) (db.Member, error) {
	if s.member.ServerID != serverID || s.member.MemberID != memberID {
		return db.Member{}, gocql.ErrNotFound
	}
	return s.member, nil
}

func (s *testServerMemberStore) IsServerModerator(
	_ context.Context, _ gocql.UUID, userID gocql.UUID,
) (bool, error) {
	return s.moderator || userID == s.moderatorID, nil
}

func (s *testServerMemberStore) GetMembers(
	context.Context, gocql.UUID,
) ([]db.Member, error) {
	s.lists++
	return []db.Member{s.member}, nil
}

func (s *testServerMemberStore) LeaveServer(
	_ context.Context, userID, serverID gocql.UUID,
) error {
	s.leaveUser, s.leaveServer = userID, serverID
	return nil
}

func (s *testServerMemberStore) RevokeMember(
	ctx context.Context, userID, serverID gocql.UUID,
) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	s.revoked = true
	return s.LeaveServer(ctx, userID, serverID)
}

func (s *testServerMemberStore) JoinServer(
	_ context.Context, args db.JoinServerArgs,
) (db.Server, error) {
	s.joinArgs = args
	return db.Server{ServerID: s.member.ServerID}, s.joinErr
}

func memberTestService(
	store *testServerMemberStore, owner gocql.UUID,
) *serverService {
	return &serverService{
		cfg: &Config{
			logger: slog.Default(), signingKey: apiTestSigningKey,
		},
		store: &testServerStore{server: db.Server{
			ServerID: store.member.ServerID, Owner: owner,
		}},
		members: store,
		cleanup: &testServerCleanupStore{called: make(chan gocql.UUID, 10)},
	}
}

func TestDeleteMemberRequiresModerator(t *testing.T) {
	owner, actor := gocql.TimeUUID(), gocql.TimeUUID()
	for _, moderator := range []bool{false, true} {
		store := &testServerMemberStore{
			member: db.Member{
				ServerID: gocql.TimeUUID(), MemberID: gocql.TimeUUID(),
			},
			moderatorID: func() gocql.UUID {
				if moderator {
					return actor
				}
				return gocql.UUID{}
			}(),
		}
		service := memberTestService(store, owner)
		_, err := service.DeleteMember(
			authenticatedTestContext(actor),
			&serversv1.DeleteMemberRequest{
				ServerId: store.member.ServerID.String(),
				MemberId: store.member.MemberID.String(),
			},
		)
		if moderator {
			if err != nil || store.leaveUser != store.member.MemberID || !store.revoked {
				t.Fatalf("moderator removal failed: %v", err)
			}
		} else if status.Code(err) != codes.PermissionDenied ||
			store.leaveUser != (gocql.UUID{}) {
			t.Fatalf("ordinary user could remove a member: %v", err)
		}
	}
}

func TestLeaveServerUsesBearerIdentity(t *testing.T) {
	owner := gocql.TimeUUID()
	store := &testServerMemberStore{member: db.Member{
		ServerID: gocql.TimeUUID(), MemberID: gocql.TimeUUID(),
	}}
	service := memberTestService(store, owner)
	req := &serversv1.LeaveServerRequest{
		ServerId: store.member.ServerID.String(),
	}
	_, err := service.LeaveServer(authenticatedTestContext(owner), req)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("owner could leave: %v", err)
	}
	_, err = service.LeaveServer(
		authenticatedTestContext(store.member.MemberID), req,
	)
	if err != nil || store.leaveUser != store.member.MemberID ||
		store.leaveServer != store.member.ServerID || store.revoked {
		t.Fatalf("wrong membership removed: %v", err)
	}
}

func TestGetMembersChecksMembership(t *testing.T) {
	owner := gocql.TimeUUID()
	store := &testServerMemberStore{member: db.Member{
		ServerID: gocql.TimeUUID(), MemberID: gocql.TimeUUID(),
		MemberName: "alice",
	}}
	service := memberTestService(store, owner)
	req := &serversv1.GetMembersRequest{
		ServerId: store.member.ServerID.String(),
	}
	_, err := service.GetMembers(
		authenticatedTestContext(gocql.TimeUUID()), req,
	)
	if status.Code(err) != codes.PermissionDenied || store.lists != 0 {
		t.Fatalf("non-member could list members: %v", err)
	}
	response, err := service.GetMembers(
		authenticatedTestContext(store.member.MemberID), req,
	)
	if err != nil || len(response.Items) != 1 ||
		response.Items[0].MemberId != store.member.MemberID.String() {
		t.Fatalf("member list failed: %v", err)
	}
}

func TestJoinServerIdentityAndErrors(t *testing.T) {
	userID := gocql.TimeUUID()
	store := &testServerMemberStore{}
	service := memberTestService(store, gocql.TimeUUID())
	service.users = &testUserStore{user: db.User{
		ID: userID, Username: "alice",
	}}
	ctx := authenticatedTestContext(userID)
	for _, test := range []struct {
		err  error
		code codes.Code
	}{
		{nil, codes.OK},
		{db.ErrAlreadyServerMember, codes.AlreadyExists},
		{db.ErrInvitationRecipient, codes.PermissionDenied},
		{db.ErrServerMemberRevoked, codes.PermissionDenied},
		{db.ErrInvitationUsed, codes.FailedPrecondition},
		{gocql.ErrNotFound, codes.NotFound},
		{errors.New("database offline"), codes.Internal},
	} {
		store.joinErr = test.err
		_, err := service.JoinServer(ctx, &serversv1.JoinServerRequest{
			ShareCode: " invitation ",
		})
		if status.Code(err) != test.code {
			t.Fatalf("got %v, want %v", err, test.code)
		}
		if store.joinArgs.UserID != userID ||
			store.joinArgs.UserName != "alice" ||
			store.joinArgs.Code != "invitation" {
			t.Fatal("join did not use authenticated identity")
		}
	}
	if _, err := service.JoinServer(ctx, nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty invitation accepted: %v", err)
	}
	_, err := service.GetMembers(context.Background(), nil)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated list accepted: %v", err)
	}
}

func TestMemberRevocationFailureIsReported(t *testing.T) {
	owner := gocql.TimeUUID()
	store := &testServerMemberStore{
		member:    db.Member{ServerID: gocql.TimeUUID(), MemberID: gocql.TimeUUID()},
		revokeErr: errors.New("database unavailable"),
	}
	service := memberTestService(store, owner)
	_, err := service.DeleteMember(authenticatedTestContext(owner), &serversv1.DeleteMemberRequest{
		ServerId: store.member.ServerID.String(), MemberId: store.member.MemberID.String(),
	})
	if status.Code(err) != codes.Internal || store.revoked || store.leaveUser != (gocql.UUID{}) {
		t.Fatalf("failed revocation reported as removal: %v", err)
	}
}
