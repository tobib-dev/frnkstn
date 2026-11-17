package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/tobib-dev/frnkstn/api/internal/messaging"
)

const (
	serverExchange       = "servers"
	serverTopic          = "servers."
	serverCleanupQueue   = "server-cleanup"
	serverCleanupTimeout = 30 * time.Second
	cleanupServerEdit    = "server_edit"
	cleanupServerDelete  = "server_delete"
	cleanupServerPurge   = "server_purge"
	cleanupMemberRemoved = "member_removed"
)

type durableBroker interface {
	Publish(context.Context, string, string, []byte) (bool, error)
}

type rabbitDurableBroker struct{ uri string }

func (b rabbitDurableBroker) Publish(
	ctx context.Context, exchange,
	routingKey string, body []byte,
) (bool, error) {
	return messaging.PublishDurable(
		ctx, exchange, routingKey,
		b.uri, serverCleanupQueue,
		serverTopic+"#", body,
	)
}

func serverRoutingKey(serverID string) string {
	return serverTopic + serverID
}

type serverCleanupEvent struct {
	ServerID  string    `json:"server_id"`
	Operation string    `json:"operation"`
	UserID    string    `json:"user_id,omitempty"`
	RemovedAt time.Time `json:"removed_at,omitempty"`
}

func (s *serverService) StartServerConsumer(
	ctx context.Context,
) error {
	if s.cfg.brokerURI == "" {
		return nil
	}
	consumer, closeConsumer, err := messaging.
		DeclareAndBindDurableConsumer(
			ctx, serverExchange,
			serverTopic+"#",
			s.cfg.brokerURI,
			serverCleanupQueue,
		)
	if err != nil {
		return err
	}
	go func() {
		defer func() {
			closeCtx, cancel := context.WithTimeout(
				context.Background(), time.Second,
			)
			defer cancel()
			if err := closeConsumer(closeCtx); err != nil {
				s.cfg.logger.Error(
					"failed to close server consumer", "error", err,
				)
			}
		}()
		for {
			delivery, err := consumer.Receive(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.cfg.logger.Error(
						"server cleanup receive failed",
						"error", err,
					)
				}
				return
			}
			if delivery == nil {
				continue
			}
			if delivery.Message() == nil {
				_ = delivery.Discard(ctx, nil)
				continue
			}
			event, err := decodeServerCleanup(
				delivery.Message().GetData(),
			)
			if err != nil {
				s.cfg.logger.Error(
					"invalid server cleanup event",
					"error", err)
				_ = delivery.Discard(ctx, nil)
				continue
			}
			workCtx, cancel := context.WithTimeout(
				ctx, serverCleanupTimeout)
			err = s.processServerCleanup(workCtx, event)
			cancel()
			if err != nil &&
				!errors.Is(err, gocql.ErrNotFound) {
				s.cfg.logger.Error("server cleanup failed",
					"server_id", event.ServerID, "operation",
					event.Operation, "error", err)
				_ = delivery.Requeue(ctx)
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				continue
			}
			if err := delivery.Accept(ctx); err != nil {
				s.cfg.logger.Error(
					"server cleanup acknowledgement failed",
					"error", err)
			}
		}
	}()
	return nil
}

func (s *serverService) handleServerByUserCleanup(
	_ context.Context, serverID gocql.UUID,
	operation string,
) {
	s.queueServerCleanup(serverCleanupEvent{
		ServerID: serverID.String(), Operation: operation,
	})
}

func (s *serverService) queueServerCleanup(
	event serverCleanupEvent) {
	// shortcut: a crash before async publication can lose cleanup; use an outbox
	// when the database change and enqueue must survive as one operation.
	go func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), serverCleanupTimeout,
		)
		defer cancel()
		if s.broker != nil {
			body, err := json.Marshal(event)
			if err == nil {
				var accepted bool
				accepted, err = s.broker.Publish(
					ctx, serverExchange,
					serverRoutingKey(event.ServerID),
					body,
				)
				if err == nil && accepted {
					return
				}
				if err == nil {
					err = fmt.Errorf("cleanup publish was not accepted")
				}
			}
			s.cfg.logger.Error(
				"server cleanup publish failed",
				"server_id", event.ServerID,
				"operation", event.Operation,
				"error", err)
		}
		fallbackCtx, cancelFallback := context.WithTimeout(
			context.Background(), serverCleanupTimeout,
		)
		defer cancelFallback()
		if err := s.processServerCleanup(
			fallbackCtx, event); err != nil {
			s.cfg.logger.Error(
				"server cleanup fallback failed",
				"server_id", event.ServerID,
				"operation", event.Operation,
				"error", err)
		}
	}()
}

func decodeServerCleanup(
	body []byte,
) (serverCleanupEvent, error) {
	var event serverCleanupEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return event, err
	}
	serverID, err := gocql.ParseUUID(event.ServerID)
	if err != nil || serverID == (gocql.UUID{}) {
		return event, fmt.Errorf("invalid cleanup server ID")
	}
	switch event.Operation {
	case cleanupServerEdit, cleanupServerDelete, cleanupServerPurge:
	case cleanupMemberRemoved:
		userID, err := gocql.ParseUUID(event.UserID)
		if err != nil || userID == (gocql.UUID{}) ||
			event.RemovedAt.IsZero() {
			return event,
				fmt.Errorf(
					"member cleanup requires user ID and removal time",
				)
		}
	default:
		return event, fmt.Errorf("unknown cleanup operation")
	}
	return event, nil
}

func (s *serverService) processServerCleanup(
	ctx context.Context, event serverCleanupEvent,
) error {
	serverID, err := gocql.ParseUUID(event.ServerID)
	if err != nil {
		return err
	}
	switch event.Operation {
	case cleanupServerEdit:
		return s.cleanup.UpdateServerLookups(ctx, serverID)
	case cleanupServerDelete:
		if err := s.cleanup.MarkServerLookupsDeleted(
			ctx, serverID); err != nil {
			return err
		}
		return s.publishServerCleanup(ctx,
			serverCleanupEvent{
				ServerID:  event.ServerID,
				Operation: cleanupServerPurge,
			})
	case cleanupServerPurge:
		return s.cleanup.PurgeServer(ctx, serverID)
	case cleanupMemberRemoved:
		userID, err := gocql.ParseUUID(event.UserID)
		if err != nil {
			return err
		}
		return s.cleanup.CleanupRemovedMember(
			ctx, userID, serverID,
			event.RemovedAt)
	default:
		return fmt.Errorf("unknown cleanup operation")
	}
}

func (s *serverService) publishServerCleanup(
	ctx context.Context, event serverCleanupEvent,
) error {
	if s.broker != nil {
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		accepted, err := s.broker.Publish(
			ctx, serverExchange, serverRoutingKey(event.ServerID), body,
		)
		if err != nil {
			return err
		}
		if !accepted {
			return fmt.Errorf("cleanup publish was not accepted")
		}
		return nil
	}
	return s.processServerCleanup(ctx, event)
}
