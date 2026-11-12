package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestInvitePublishesToRecipientAndFallsBack(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "publish", true: "fallback"}[fallback], func(t *testing.T) {
			owner, recipient := gocql.TimeUUID(), gocql.TimeUUID()
			members := &testServerMemberStore{member: db.Member{ServerID: gocql.TimeUUID()}}
			service := memberTestService(members, owner)
			service.users = &testUserStore{users: map[gocql.UUID]db.User{recipient: {ID: recipient}}}
			store := &testNotificationStore{inviteAdded: make(chan struct{}, 1)}
			broker := &testNotificationBroker{publishFinished: make(chan struct{}, 1)}
			if fallback {
				broker.publishErr = errors.New("offline")
			}
			service.n10n = &notificationService{cfg: service.cfg, store: store, broker: broker}
			_, err := service.InviteMember(authenticatedTestContext(owner), &serversv1.InviteMemberRequest{
				ServerId: members.member.ServerID.String(), MemberId: recipient.String(),
			})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-broker.publishFinished:
			case <-time.After(time.Second):
				t.Fatal("notification not published")
			}
			if broker.exchange != notificationExchange ||
				broker.routingKey != notificationRoutingKey(
					recipient.String(),
				) {
				t.Fatal("wrong notification route")
			}
			event := &ntfnV1.SubscribeResponse{}
			if err := proto.Unmarshal(broker.body, event); err != nil {
				t.Fatal(err)
			}
			invite := event.GetInvite()
			if invite.GetRecipientId() != recipient.String() || invite.GetShareCode() != members.invites[0].Code {
				t.Fatal("published invitation does not match saved invitation")
			}
			if fallback {
				select {
				case <-store.inviteAdded:
				case <-time.After(time.Second):
					t.Fatal("fallback did not store invitation")
				}
			} else if err := service.n10n.addInviteNotification(context.Background(), invite); err != nil {
				t.Fatal(err)
			}
			stream := &testNotificationStream{ctx: authenticatedTestContext(recipient)}
			if err := service.n10n.Subscribe(&ntfnV1.SubscribeRequest{}, stream); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range stream.items {
				if item.GetInvite().GetShareCode() == invite.GetShareCode() {
					found = true
				}
			}
			if !found {
				t.Fatal("invitation missing from subscription")
			}
			_, err = service.n10n.UpdateInviteN10N(authenticatedTestContext(recipient),
				&ntfnV1.UpdateInviteN10NRequest{Id: invite.GetId()})
			if err != nil {
				t.Fatal(err)
			}
			if store.updatedInvite != [2]gocql.UUID{recipient, store.invite.NotificationID} {
				t.Fatal("update must use bearer identity and notification ID")
			}
			_, err = service.n10n.UpdateInviteN10N(context.Background(),
				&ntfnV1.UpdateInviteN10NRequest{Id: invite.GetId()})
			if status.Code(err) != codes.Unauthenticated {
				t.Fatal("missing auth accepted")
			}
		})
	}
}
