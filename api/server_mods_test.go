package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testServerModsStore struct {
	mod             db.Mod
	err             error
	calls           int
	serverID, modID gocql.UUID
}

func (s *testServerModsStore) AddMod(_ context.Context, serverID, modID gocql.UUID) (db.Mod, error) {
	s.calls++
	s.serverID, s.modID = serverID, modID
	return s.mod, s.err
}
func (s *testServerModsStore) GetMod(ctx context.Context, serverID, modID gocql.UUID) (db.Mod, error) {
	return s.AddMod(ctx, serverID, modID)
}
func (s *testServerModsStore) GetMods(_ context.Context, serverID gocql.UUID) ([]db.Mod, error) {
	s.calls++
	s.serverID = serverID
	return []db.Mod{s.mod}, s.err
}
func (s *testServerModsStore) DeleteMod(ctx context.Context, serverID, modID gocql.UUID) error {
	_, err := s.AddMod(ctx, serverID, modID)
	return err
}

func TestModeratorRPCPermissions(t *testing.T) {
	owner, member, moderator, outsider := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	serverID := gocql.TimeUUID()
	members := &testServerMemberStore{
		member: db.Member{ServerID: serverID, MemberID: member}, moderatorID: moderator,
	}
	s := memberTestService(members, owner)
	mods := &testServerModsStore{mod: db.Mod{ServerID: serverID, ModID: member, ModName: "alice"}}
	s.mods = mods
	rpcs := []struct {
		name  string
		write bool
		call  func(context.Context) error
	}{
		{"add", true, func(ctx context.Context) error {
			res, err := s.AddMod(ctx, &serversv1.AddModRequest{
				ServerId: serverID.String(), ModUserId: member.String(),
			})
			if err == nil && (res.Item.ModId != member.String() || res.Item.ModName != "alice") {
				t.Fatal("wrong added moderator")
			}
			return err
		}},
		{"get", false, func(ctx context.Context) error {
			res, err := s.GetMod(ctx, &serversv1.GetModRequest{ServerId: serverID.String(), ModId: member.String()})
			if err == nil && res.Item.ModName != "alice" {
				t.Fatal("wrong moderator")
			}
			return err
		}},
		{"list", false, func(ctx context.Context) error {
			res, err := s.GetMods(ctx, &serversv1.GetModsRequest{ServerId: serverID.String()})
			if err == nil && (len(res.Items) != 1 || res.Items[0].ModName != "alice") {
				t.Fatal("wrong moderators")
			}
			return err
		}},
		{"delete", true, func(ctx context.Context) error {
			_, err := s.DeleteMod(ctx, &serversv1.DeleteModRequest{
				ServerId: serverID.String(), ModId: member.String(),
			})
			return err
		}},
	}
	for _, rpc := range rpcs {
		t.Run(rpc.name, func(t *testing.T) {
			for _, actor := range []gocql.UUID{owner, member, moderator, outsider, {}} {
				ctx := context.Background()
				if actor != (gocql.UUID{}) {
					ctx = authenticatedTestContext(actor)
				}
				want := codes.OK
				if actor == outsider || (rpc.write && actor != owner) {
					want = codes.PermissionDenied
				}
				if actor == (gocql.UUID{}) {
					want = codes.Unauthenticated
				}
				before := mods.calls
				err := rpc.call(ctx)
				if status.Code(err) != want {
					t.Fatalf("actor %v: got %v, want %v", actor, err, want)
				}
				if want != codes.OK && mods.calls != before {
					t.Fatal("unauthorized store access")
				}
				if want == codes.OK && (mods.serverID != serverID || (rpc.name != "list" && mods.modID != member)) {
					t.Fatal("wrong query identity")
				}
			}
			for _, failure := range []struct {
				err  error
				code codes.Code
			}{
				{gocql.ErrNotFound, codes.NotFound}, {errors.New("offline"), codes.Internal},
			} {
				mods.err = failure.err
				if err := rpc.call(authenticatedTestContext(owner)); status.Code(err) != failure.code {
					t.Fatalf("wrong store error: %v", err)
				}
			}
			mods.err = nil
		})
	}
}

func TestModeratorMutationValidation(t *testing.T) {
	owner, member, serverID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	s := memberTestService(&testServerMemberStore{member: db.Member{ServerID: serverID}}, owner)
	mods := &testServerModsStore{}
	s.mods = mods
	ctx := authenticatedTestContext(owner)
	for _, test := range []struct {
		server, user string
		code         codes.Code
	}{
		{"bad", member.String(), codes.InvalidArgument},
		{serverID.String(), "bad", codes.InvalidArgument},
		{serverID.String(), owner.String(), codes.AlreadyExists},
	} {
		_, err := s.AddMod(ctx, &serversv1.AddModRequest{ServerId: test.server, ModUserId: test.user})
		if status.Code(err) != test.code {
			t.Fatalf("add validation: %v", err)
		}
	}
	for _, test := range []struct {
		mod, user string
		code      codes.Code
	}{
		{"bad", "", codes.InvalidArgument},
		{member.String(), "bad", codes.InvalidArgument},
		{member.String(), owner.String(), codes.InvalidArgument},
		{owner.String(), "", codes.PermissionDenied},
	} {
		_, err := s.DeleteMod(ctx, &serversv1.DeleteModRequest{
			ServerId: serverID.String(), ModId: test.mod, ModUserId: test.user,
		})
		if status.Code(err) != test.code {
			t.Fatalf("delete validation: %v", err)
		}
	}
	if mods.calls != 0 {
		t.Fatal("invalid request reached store")
	}
}

func TestModeratorCannotRemoveModerator(t *testing.T) {
	owner, actor := gocql.TimeUUID(), gocql.TimeUUID()
	members := &testServerMemberStore{
		member: db.Member{ServerID: gocql.TimeUUID(), MemberID: gocql.TimeUUID()}, moderator: true,
	}
	s := memberTestService(members, owner)
	req := &serversv1.DeleteMemberRequest{
		ServerId: members.member.ServerID.String(), MemberId: members.member.MemberID.String(),
	}
	_, err := s.DeleteMember(authenticatedTestContext(actor), req)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("moderator could remove peer: %v", err)
	}
	if members.leaveUser != (gocql.UUID{}) {
		t.Fatal("denied removal reached store")
	}
	if _, err := s.DeleteMember(authenticatedTestContext(owner), req); err != nil {
		t.Fatalf("owner could not remove moderator: %v", err)
	}
}

func TestServerRPCLogsRejectedRequests(t *testing.T) {
	var output bytes.Buffer
	s := &serverService{cfg: &Config{
		logger: slog.New(slog.NewJSONHandler(&output, nil)),
	}}
	ctx := context.Background()
	for _, test := range []struct {
		operation string
		call      func() error
	}{
		{"AddMod", func() error { _, err := s.AddMod(ctx, nil); return err }},
		{"GetMod", func() error { _, err := s.GetMod(ctx, nil); return err }},
		{"GetMods", func() error { _, err := s.GetMods(ctx, nil); return err }},
		{"DeleteMod", func() error { _, err := s.DeleteMod(ctx, nil); return err }},
		{"CreateServer", func() error { _, err := s.CreateServer(ctx, nil); return err }},
	} {
		t.Run(test.operation, func(t *testing.T) {
			output.Reset()
			err := test.call()
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("unexpected error: %v", err)
			}
			var entry map[string]any
			if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["operation"] != test.operation || entry["code"] != "Unauthenticated" ||
				entry["level"] != "WARN" || entry["error"] == nil {
				t.Fatalf("missing error context: %v", entry)
			}
		})
	}
}
