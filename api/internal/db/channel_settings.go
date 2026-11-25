package db

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type SettingStore interface {
	AddSlowMode(context.Context, ChannelSetting) (ChannelSetting, error)
	GetCoolDownTime(context.Context, gocql.UUID, gocql.UUID) (*int, error)
	GetChannelWait(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) (time.Duration, error)
}

type ChannelSetting struct {
	ServerID        gocql.UUID
	ChannelID       gocql.UUID
	SlowModeSeconds *int
	UpdatedBy       gocql.UUID
	UpdatedAt       time.Time
}

var channelSettingTable = table.New(table.Metadata{
	Name:    "channel_settings",
	Columns: []string{"server_id", "channel_id", "slow_mode_seconds", "updated_by", "updated_at"},
	PartKey: []string{"server_id"},
	SortKey: []string{"channel_id"},
})

func (db *DB) AddSlowMode(ctx context.Context, setting ChannelSetting) (ChannelSetting, error) {
	err := db.Session.Query(channelSettingTable.Insert()).WithContext(ctx).BindStruct(setting).ExecRelease()
	return setting, err
}

func (db *DB) GetCoolDownTime(ctx context.Context, serverID, channelID gocql.UUID) (*int, error) {
	setting := ChannelSetting{ServerID: serverID, ChannelID: channelID}
	err := db.Session.Query(channelSettingTable.Get()).WithContext(ctx).BindStruct(setting).GetRelease(&setting)
	if errors.Is(err, gocql.ErrNotFound) {
		return nil, nil
	}
	return setting.SlowModeSeconds, err
}

var ErrSlowMode = errors.New("channel slowmode cooldown is active")

var channelWriteTable = table.New(table.Metadata{
	Name:    "channel_write_cooldowns",
	Columns: []string{"server_id", "channel_id", "user_id", "last_write"},
	PartKey: []string{"server_id", "channel_id", "user_id"},
})

func (db *DB) GetChannelWait(ctx context.Context, serverID, channelID, userID gocql.UUID) (time.Duration, error) {
	seconds, err := db.GetCoolDownTime(ctx, serverID, channelID)
	if err != nil || seconds == nil || *seconds == 0 {
		return 0, err
	}
	var previous struct{ LastWrite time.Time }
	err = db.Session.Query(channelWriteTable.Get("last_write")).WithContext(ctx).
		BindMap(qb.M{"server_id": serverID, "channel_id": channelID, "user_id": userID}).GetRelease(&previous)
	if errors.Is(err, gocql.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return max(0, time.Until(previous.LastWrite.Add(time.Duration(*seconds)*time.Second))), nil
}

func (db *DB) enforceSlowMode(ctx context.Context, serverID, channelID, userID gocql.UUID) error {
	seconds, err := db.GetCoolDownTime(ctx, serverID, channelID)
	if err != nil || seconds == nil || *seconds == 0 {
		return err
	}
	values := qb.M{"server_id": serverID, "channel_id": channelID, "user_id": userID}
	var previous struct{ LastWrite time.Time }
	err = db.Session.Query(channelWriteTable.Get("last_write")).WithContext(ctx).BindMap(values).GetRelease(&previous)
	now := time.Now().UTC().Truncate(time.Millisecond)
	values["last_write"] = now
	var applied bool
	switch {
	case errors.Is(err, gocql.ErrNotFound):
		applied, err = qb.Insert(channelWriteTable.Name()).Columns(channelWriteTable.Metadata().Columns...).Unique().
			Query(*db.Session).WithContext(ctx).BindMap(values).ExecCASRelease()
	case err != nil:
		return err
	default:
		if now.Before(previous.LastWrite.Add(time.Duration(*seconds) * time.Second)) {
			return ErrSlowMode
		}
		values["previous_write"] = previous.LastWrite
		applied, err = qb.Update(channelWriteTable.Name()).Set("last_write").
			Where(qb.Eq("server_id"), qb.Eq("channel_id"), qb.Eq("user_id")).
			If(qb.EqNamed("last_write", "previous_write")).Query(*db.Session).
			WithContext(ctx).BindMap(values).ExecCASRelease()
	}
	if err != nil {
		return err
	}
	if !applied {
		return ErrSlowMode
	}
	return nil
}
