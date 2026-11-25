package main

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/gocql/gocql"
	moderationsv1 "github.com/tobib-dev/frnkstn-proto/moderations"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testSettingStore struct {
	db.SettingStore
	setting db.ChannelSetting
	calls   int
	wait    time.Duration
}

func (s *testSettingStore) GetCoolDownTime(context.Context, gocql.UUID, gocql.UUID) (*int, error) {
	return s.setting.SlowModeSeconds, nil
}

func (s *testSettingStore) GetChannelWait(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) (time.Duration, error) {
	return s.wait, nil
}

func (s *testSettingStore) AddSlowMode(_ context.Context, setting db.ChannelSetting) (db.ChannelSetting, error) {
	s.setting, s.calls = setting, s.calls+1
	return setting, nil
}

func TestSetSlowModePermissionsAndDisable(t *testing.T) {
	owner, mod, member := gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()
	serverID, channelID := gocql.TimeUUID(), gocql.TimeUUID()
	members := &testServerMemberStore{member: db.Member{ServerID: serverID, MemberID: member}, moderatorID: mod}
	service := memberTestService(members, owner)
	service.channels = &testChannelStore{}
	settings := &testSettingStore{}
	service.settings = settings
	req := &moderationsv1.SetSlowModeRequest{ServerId: serverID.String(), ChannelId: channelID.String(), CooldownSeconds: 30}
	if _, err := service.SetSlowMode(authenticatedTestContext(member), req); status.Code(err) != codes.PermissionDenied || settings.calls != 0 {
		t.Fatalf("member could change slowmode: %v", err)
	}
	for _, actor := range []gocql.UUID{owner, mod} {
		response, err := service.SetSlowMode(authenticatedTestContext(actor), req)
		if err != nil || response.GetCooldownSeconds() != 30 || settings.setting.SlowModeSeconds == nil ||
			*settings.setting.SlowModeSeconds != 30 || settings.setting.UpdatedBy != actor || settings.setting.UpdatedAt.IsZero() {
			t.Fatalf("could not set slowmode: response=%v setting=%+v error=%v", response, settings.setting, err)
		}
	}
	req.CooldownSeconds = 0
	if _, err := service.SetSlowMode(authenticatedTestContext(mod), req); err != nil ||
		settings.setting.SlowModeSeconds == nil || *settings.setting.SlowModeSeconds != 0 {
		t.Fatalf("could not disable slowmode: %v", err)
	}
	req.CooldownSeconds = math.MaxUint32
	if _, err := service.SetSlowMode(authenticatedTestContext(mod), req); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("accepted cooldown that overflows CQL INT: %v", err)
	}
	if status.Code(service.serverMessageError("post message", db.ErrSlowMode)) != codes.ResourceExhausted {
		t.Fatal("cooldown rejection must be ResourceExhausted")
	}
}

func TestGetSlowModeIncludesRemainingWait(t *testing.T) {
	owner, member := gocql.TimeUUID(), gocql.TimeUUID()
	serverID, channelID := gocql.TimeUUID(), gocql.TimeUUID()
	service := memberTestService(&testServerMemberStore{member: db.Member{ServerID: serverID, MemberID: member}}, owner)
	service.channels = &testChannelStore{}
	seconds := 30
	settings := &testSettingStore{setting: db.ChannelSetting{SlowModeSeconds: &seconds}, wait: 1500*time.Millisecond + 1}
	service.settings = settings
	req := &moderationsv1.GetSlowModeRequest{ServerId: serverID.String(), ChannelId: channelID.String()}
	response, err := service.GetSlowMode(authenticatedTestContext(member), req)
	if err != nil || response.GetCooldownSeconds() != 30 || response.GetRemainingMillis() != 1501 {
		t.Fatalf("unexpected cooldown status: %v, error=%v", response, err)
	}
	settings.setting.SlowModeSeconds = nil
	response, err = service.GetSlowMode(authenticatedTestContext(member), req)
	if err != nil || response.GetCooldownSeconds() != 0 || response.GetRemainingMillis() != 0 {
		t.Fatalf("unset slowmode returned wait: %v %v", response, err)
	}
}
