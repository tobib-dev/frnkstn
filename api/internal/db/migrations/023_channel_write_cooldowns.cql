CREATE TABLE frnkstn.channel_write_cooldowns (
    server_id TIMEUUID,
    channel_id TIMEUUID,
    user_id TIMEUUID,
    last_write TIMESTAMP,
    PRIMARY KEY ((server_id, channel_id, user_id))
);
