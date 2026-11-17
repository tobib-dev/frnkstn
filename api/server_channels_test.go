package main

import (
	"context"
	"errors"
	"testing"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testChannelStore struct {
	channel db.Channel
	err     error
	calls   int
	empty   bool
}

func (s *testChannelStore) AddChannel(_ context.Context, channel db.Channel) (db.Channel, error) {
	s.calls++
	s.channel = channel
	return channel, s.err
}
func (s *testChannelStore) GetChannel(_ context.Context, serverID, channelID gocql.UUID) (db.Channel, error) {
	s.calls++
	s.channel.ServerID, s.channel.ChannelID = serverID, channelID
	return s.channel, s.err
}
func (s *testChannelStore) GetChannels(_ context.Context, serverID gocql.UUID) ([]db.Channel, error) {
	s.calls++
	s.channel.ServerID = serverID
	if s.empty {
		return nil, s.err
	}
	return []db.Channel{s.channel}, s.err
}
func (s *testChannelStore) EditChannel(ctx context.Context, args db.EditChannelParams) (db.Channel, error) {
	return s.AddChannel(ctx, db.Channel{
		ServerID: args.ServerID, ChannelID: args.ChannelID, ChannelName: args.ChannelName,
	})
}
func (s *testChannelStore) DeleteChannel(ctx context.Context, serverID, channelID gocql.UUID) error {
	_, err := s.GetChannel(ctx, serverID, channelID)
	return err
}

func TestChannelPermissionsAndErrors(t *testing.T) {
	owner, mod, member, outsider := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	serverID, channelID := gocql.TimeUUID(), gocql.TimeUUID()
	members := &testServerMemberStore{
		member: db.Member{ServerID: serverID, MemberID: member}, moderatorID: mod,
	}
	service := memberTestService(members, owner)
	store := &testChannelStore{channel: db.Channel{ChannelName: "general"}}
	service.channels = store
	item := &serversv1.ChannelItem{
		ServerId: serverID.String(), ChannelId: channelID.String(), ChannelName: " general ",
	}
	for _, test := range []struct {
		name  string
		write bool
		call  func(context.Context) error
	}{
		{"add", true, func(ctx context.Context) error {
			response, err := service.AddChannel(ctx, &serversv1.AddChannelRequest{
				ServerId: serverID.String(), ChannelName: " general ",
			})
			if err == nil && (response.Item.ChannelName != "general" || store.channel.ChannelID == (gocql.UUID{})) {
				t.Fatal("channel name or generated ID missing")
			}
			return err
		}},
		{"get", false, func(ctx context.Context) error {
			response, err := service.GetChannel(ctx, &serversv1.GetChannelRequest{
				ServerId: serverID.String(), ChannelId: channelID.String(),
			})
			if err == nil && (response.Item.ChannelId != channelID.String() ||
				response.Item.ChannelName != "general") {
				t.Fatal("wrong channel returned")
			}
			return err
		}},
		{"list", false, func(ctx context.Context) error {
			response, err := service.GetChannels(ctx, &serversv1.GetChannelsRequest{ServerId: serverID.String()})
			if err == nil && (len(response.Items) != 1 || response.Items[0].ServerId != serverID.String()) {
				t.Fatal("wrong channels returned")
			}
			return err
		}},
		{"edit", true, func(ctx context.Context) error {
			response, err := service.EditChannel(ctx, &serversv1.EditChannelRequest{Item: item})
			if err == nil && response.Item.ChannelName != "general" {
				t.Fatal("edit did not trim name")
			}
			return err
		}},
		{"delete", true, func(ctx context.Context) error {
			_, err := service.DeleteChannel(ctx, &serversv1.DeleteChannelRequest{
				ServerId: serverID.String(), ChannelId: channelID.String(),
			})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, actor := range []gocql.UUID{owner, mod, member, outsider, {}} {
				ctx := context.Background()
				if actor != (gocql.UUID{}) {
					ctx = authenticatedTestContext(actor)
				}
				want := codes.OK
				if actor == outsider || (test.write && actor == member) {
					want = codes.PermissionDenied
				}
				if actor == (gocql.UUID{}) {
					want = codes.Unauthenticated
				}
				calls := store.calls
				err := test.call(ctx)
				if status.Code(err) != want {
					t.Fatalf("actor %v: got %v, want %v", actor, err, want)
				}
				if want != codes.OK && store.calls != calls {
					t.Fatal("unauthorized query executed")
				}
				if want == codes.OK && (store.channel.ServerID != serverID ||
					(test.name != "add" && test.name != "list" && store.channel.ChannelID != channelID)) {
					t.Fatal("query used wrong server/channel identity")
				}
			}
			for _, failure := range []struct {
				err  error
				code codes.Code
			}{
				{gocql.ErrNotFound, codes.NotFound}, {errors.New("database unavailable"), codes.Internal},
			} {
				store.err = failure.err
				if err := test.call(authenticatedTestContext(owner)); status.Code(err) != failure.code {
					t.Fatalf("wrong store error: %v", err)
				}
			}
			store.err = nil
			serverStore := service.store.(*testServerStore)
			serverStore.getErr = gocql.ErrNotFound
			calls := store.calls
			err := test.call(authenticatedTestContext(owner))
			if status.Code(err) != codes.NotFound || store.calls != calls {
				t.Fatalf("missing server reached channel query: %v", err)
			}
			serverStore.getErr = nil
		})
	}
	store.empty = true
	response, err := service.GetChannels(authenticatedTestContext(member),
		&serversv1.GetChannelsRequest{ServerId: serverID.String()})
	if err != nil || len(response.Items) != 0 {
		t.Fatalf("empty list failed: %v", err)
	}
}

func TestChannelRequestValidation(t *testing.T) {
	owner, serverID, channelID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	service := memberTestService(&testServerMemberStore{member: db.Member{ServerID: serverID}}, owner)
	store := &testChannelStore{}
	service.channels = store
	ctx := authenticatedTestContext(owner)
	for _, call := range []func() error{
		func() error { _, err := service.AddChannel(ctx, nil); return err },
		func() error {
			_, err := service.AddChannel(ctx, &serversv1.AddChannelRequest{
				ServerId: serverID.String(), ChannelName: "  ",
			})
			return err
		},
		func() error { _, err := service.EditChannel(ctx, nil); return err },
		func() error {
			_, err := service.EditChannel(ctx, &serversv1.EditChannelRequest{Item: &serversv1.ChannelItem{
				ServerId: serverID.String(), ChannelId: channelID.String(), ChannelName: "  ",
			}})
			return err
		},
		func() error {
			_, err := service.GetChannel(ctx, &serversv1.GetChannelRequest{
				ServerId: serverID.String(), ChannelId: "bad",
			})
			return err
		},
		func() error {
			_, err := service.GetChannels(ctx, &serversv1.GetChannelsRequest{ServerId: "bad"})
			return err
		},
		func() error {
			_, err := service.DeleteChannel(ctx, &serversv1.DeleteChannelRequest{ServerId: serverID.String()})
			return err
		},
	} {
		if err := call(); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid request accepted: %v", err)
		}
	}
	if store.calls != 0 {
		t.Fatal("invalid request reached channel store")
	}
}
