package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ChannelStore interface {
	AddChannel(context.Context, Channel) (Channel, error)
	GetChannel(context.Context, gocql.UUID, gocql.UUID) (Channel, error)
	GetChannels(context.Context, gocql.UUID) ([]Channel, error)
	EditChannel(context.Context, EditChannelParams) (Channel, error)
	DeleteChannel(context.Context, gocql.UUID, gocql.UUID) error
}

type Channel struct {
	ServerID    gocql.UUID
	ChannelID   gocql.UUID
	ChannelName string
}

type EditChannelParams struct {
	ServerID    gocql.UUID
	ChannelID   gocql.UUID
	ChannelName string
}

var serverChannel = table.New(table.Metadata{
	Name:    "server_channels",
	Columns: []string{"server_id", "channel_id", "channel_name"},
	PartKey: []string{"server_id"},
	SortKey: []string{"channel_id"},
})

func (db *DB) AddChannel(
	ctx context.Context,
	channel Channel,
) (Channel, error) {
	err := db.Session.Query(
		serverChannel.Insert(),
	).
		WithContext(ctx).
		BindStruct(channel).
		ExecRelease()
	return channel, err
}

func (db *DB) GetChannel(
	ctx context.Context,
	serverID, channelID gocql.UUID,
) (Channel, error) {
	channel := Channel{
		ServerID:  serverID,
		ChannelID: channelID,
	}
	err := db.Session.Query(
		serverChannel.Get(),
	).
		WithContext(ctx).
		BindStruct(channel).
		GetRelease(&channel)
	return channel, err
}

func (db *DB) GetChannels(
	ctx context.Context,
	serverID gocql.UUID,
) ([]Channel, error) {
	channels := []Channel{}
	err := db.Session.Query(
		serverChannel.Select(),
	).
		WithContext(ctx).
		BindMap(qb.M{"server_id": serverID}).
		SelectRelease(&channels)
	return channels, err
}

func (db *DB) EditChannel(
	ctx context.Context,
	args EditChannelParams,
) (Channel, error) {
	applied, err := qb.Update(
		serverChannel.Name()).
		Set("channel_name").
		Where(
			qb.Eq("server_id"),
			qb.Eq("channel_id")).
		Existing().
		Query(*db.Session).
		WithContext(ctx).
		BindStruct(args).
		ExecCASRelease()
	if err != nil {
		return Channel{}, err
	}
	if !applied {
		return Channel{}, gocql.ErrNotFound
	}
	return Channel{
		ServerID:    args.ServerID,
		ChannelID:   args.ChannelID,
		ChannelName: args.ChannelName,
	}, nil
}

func (db *DB) DeleteChannel(
	ctx context.Context,
	serverID, channelID gocql.UUID,
) error {
	return db.Session.Query(
		serverChannel.Delete(),
	).WithContext(ctx).
		BindStruct(Channel{
			ServerID:  serverID,
			ChannelID: channelID}).
		ExecRelease()
}
