CREATE TABLE IF NOT EXISTS frnkstn.server_channels(
    server_id TIMEUUID,
    channel_id TIMEUUID,
    channel_name TEXT,
    PRIMARY KEY((server_id), channel_id)
);
