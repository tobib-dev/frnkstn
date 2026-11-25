CREATE TABLE frnkstn.channel_settings (
    server_id TIMEUUID,
    channel_id TIMEUUID,
    slow_mode_seconds INT,
    updated_by TIMEUUID,
    updated_at TIMESTAMP,
    PRIMARY KEY (server_id, channel_id)
);
