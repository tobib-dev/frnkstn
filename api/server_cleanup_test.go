package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

type testServerCleanupStore struct {
	called chan gocql.UUID
}

type operationCleanupStore struct {
	testServerCleanupStore
	operations []string
	markErr    error
}

func (s *operationCleanupStore) UpdateServerLookups(
	context.Context, gocql.UUID,
) error {
	s.operations = append(s.operations, cleanupServerEdit)
	return nil
}

func (s *operationCleanupStore) MarkServerLookupsDeleted(
	context.Context, gocql.UUID,
) error {
	s.operations = append(s.operations, cleanupServerDelete)
	return s.markErr
}

func (s *operationCleanupStore) PurgeServer(
	context.Context, gocql.UUID,
) error {
	s.operations = append(s.operations, cleanupServerPurge)
	return nil
}

func (s *operationCleanupStore) CleanupRemovedMember(
	context.Context, gocql.UUID, gocql.UUID, time.Time,
) error {
	s.operations = append(s.operations, cleanupMemberRemoved)
	return nil
}

func TestServerCleanupDispatchAndDeleteStages(t *testing.T) {
	for _, operation := range []string{
		cleanupServerEdit, cleanupServerDelete, cleanupMemberRemoved,
	} {
		store := &operationCleanupStore{}
		service := &serverService{cleanup: store}
		event := serverCleanupEvent{
			ServerID: gocql.TimeUUID().String(), Operation: operation,
			UserID: gocql.TimeUUID().String(), RemovedAt: time.Now(),
		}
		if err := service.processServerCleanup(context.Background(), event); err != nil {
			t.Fatal(err)
		}
		want := []string{operation}
		if operation == cleanupServerDelete {
			want = append(want, cleanupServerPurge)
		}
		if !reflect.DeepEqual(store.operations, want) {
			t.Fatalf("operations=%v want=%v", store.operations, want)
		}
	}
	store := &operationCleanupStore{markErr: errors.New("write failed")}
	service := &serverService{cleanup: store}
	err := service.processServerCleanup(context.Background(), serverCleanupEvent{
		ServerID: gocql.TimeUUID().String(), Operation: cleanupServerDelete,
	})
	if err == nil || len(store.operations) != 1 {
		t.Fatal("purge ran before deletion propagation succeeded")
	}
}

func (s *testServerCleanupStore) UpdateServerLookups(
	_ context.Context, serverID gocql.UUID,
) error {
	s.called <- serverID
	return nil
}

func (s *testServerCleanupStore) MarkServerLookupsDeleted(
	ctx context.Context, id gocql.UUID,
) error {
	return s.UpdateServerLookups(ctx, id)
}

func (s *testServerCleanupStore) PurgeServer(
	ctx context.Context, id gocql.UUID,
) error {
	return s.UpdateServerLookups(ctx, id)
}

func (s *testServerCleanupStore) CleanupRemovedMember(
	ctx context.Context, userID, serverID gocql.UUID, removedAt time.Time,
) error {
	return s.UpdateServerLookups(ctx, serverID)
}

type testServerCleanupBroker struct {
	err    error
	called chan serverCleanupEvent
}

func (b *testServerCleanupBroker) Publish(
	_ context.Context, exchange, key string, body []byte,
) (bool, error) {
	var event serverCleanupEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return false, err
	}
	if exchange != serverExchange || key != serverRoutingKey(event.ServerID) {
		return false, errors.New("incorrect cleanup route")
	}
	b.called <- event
	return b.err == nil, b.err
}

func TestServerCleanupPublishAndFallback(t *testing.T) {
	for _, failed := range []bool{false, true} {
		broker := &testServerCleanupBroker{called: make(chan serverCleanupEvent, 1)}
		if failed {
			broker.err = errors.New("broker offline")
		}
		store := &testServerCleanupStore{called: make(chan gocql.UUID, 1)}
		service := &serverService{
			cfg: &Config{logger: slog.Default()}, broker: broker, cleanup: store,
		}
		serverID := gocql.TimeUUID()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		service.handleServerByUserCleanup(ctx, serverID, cleanupServerEdit)
		select {
		case event := <-broker.called:
			if event.ServerID != serverID.String() || event.Operation != cleanupServerEdit {
				t.Fatal("wrong server published")
			}
		case <-time.After(time.Second):
			t.Fatal("cleanup not published")
		}
		if failed {
			select {
			case id := <-store.called:
				if id != serverID {
					t.Fatal("wrong fallback server")
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup fallback not called")
			}
		}
	}
}

func TestDecodeServerCleanupRejectsInvalidEvents(t *testing.T) {
	for _, body := range []string{"invalid", "{}", `{"server_id":"bad"}`} {
		if _, err := decodeServerCleanup([]byte(body)); err == nil {
			t.Fatalf("accepted invalid event: %s", body)
		}
	}
	id := gocql.TimeUUID()
	body, err := json.Marshal(serverCleanupEvent{
		ServerID: id.String(), Operation: cleanupServerEdit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodeServerCleanup(body); err != nil || decoded.ServerID != id.String() {
		t.Fatalf("valid event rejected: %v", err)
	}
}

func TestPurgePublishFailureKeepsDeleteJobRetryable(t *testing.T) {
	store := &operationCleanupStore{}
	broker := &testServerCleanupBroker{
		err:    errors.New("broker unavailable"),
		called: make(chan serverCleanupEvent, 1),
	}
	service := &serverService{cleanup: store, broker: broker}
	err := service.processServerCleanup(context.Background(), serverCleanupEvent{
		ServerID: gocql.TimeUUID().String(), Operation: cleanupServerDelete,
	})
	if err == nil {
		t.Fatal("failed purge publication must fail the delete job")
	}
	if !reflect.DeepEqual(store.operations, []string{cleanupServerDelete}) {
		t.Fatalf("purge ran without durable publication: %v", store.operations)
	}
	if event := <-broker.called; event.Operation != cleanupServerPurge {
		t.Fatalf("unexpected published operation: %s", event.Operation)
	}
}
