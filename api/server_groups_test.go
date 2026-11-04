package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEditServerRejectsMissingItem(t *testing.T) {
	for _, req := range []*serversv1.EditServerRequest{nil, {}} {
		service := &serverService{}
		response, err := service.EditServer(context.Background(), req)
		if response != nil || status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument without a response, got response=%v error=%v", response, err)
		}
	}
}

type testServerStore struct {
	db.ServerStore
	server  db.Server
	getErr  error
	editErr error
	reads   int
	edits   int
}

func (s *testServerStore) GetServer(context.Context, gocql.UUID) (db.Server, error) {
	s.reads++
	return s.server, s.getErr
}

func (s *testServerStore) EditServer(_ context.Context, id gocql.UUID, params db.EditServerParams) (db.Server, error) {
	s.edits++
	return db.Server{ServerID: id, ServerName: params.ServerName, ServerBio: params.ServerBio, Owner: params.Owner}, s.editErr
}

func TestEditServerPermissionsAndErrors(t *testing.T) {
	owner, other, serverID := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	for _, test := range []struct {
		name    string
		user    gocql.UUID
		getErr  error
		editErr error
		code    codes.Code
		edits   int
	}{
		{"owner", owner, nil, nil, codes.OK, 1},
		{"non-owner with unchanged owner", other, nil, nil, codes.PermissionDenied, 0},
		{"missing server", owner, gocql.ErrNotFound, nil, codes.NotFound, 0},
		{"lookup failure", owner, errors.New("database unavailable"), nil, codes.Internal, 0},
		{"write failure", owner, nil, errors.New("write failed"), codes.Internal, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testServerStore{server: db.Server{ServerID: serverID, Owner: owner, ServerName: "name"}, getErr: test.getErr, editErr: test.editErr}
			service := &serverService{cfg: &Config{logger: slog.Default(), signingKey: apiTestSigningKey}, store: store}
			response, err := service.EditServer(authenticatedTestContext(test.user), &serversv1.EditServerRequest{Item: &serversv1.ServerItem{ServerId: serverID.String(), Owner: owner.String(), ServerName: "name", ServerBio: "updated"}})
			if status.Code(err) != test.code || store.edits != test.edits {
				t.Fatalf("code=%v edits=%d; want code=%v edits=%d", status.Code(err), store.edits, test.code, test.edits)
			}
			if err != nil && response != nil {
				t.Fatal("error returned with success response")
			}
		})
	}
}

func TestEditServerRejectsUnauthenticatedCallerBeforeLookup(t *testing.T) {
	store := &testServerStore{}
	service := &serverService{cfg: &Config{logger: slog.Default(), signingKey: apiTestSigningKey}, store: store}
	_, err := service.EditServer(context.Background(), &serversv1.EditServerRequest{Item: &serversv1.ServerItem{}})
	if status.Code(err) != codes.Unauthenticated || store.reads != 0 || store.edits != 0 {
		t.Fatalf("err=%v reads=%d edits=%d", err, store.reads, store.edits)
	}
}
