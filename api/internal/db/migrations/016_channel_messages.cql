CREATE TABLE IF NOT EXISTS frnkstn.channel_messages(
    server_id TIMEUUID,
    channel_id TIMEUUID,
    message_id TIMEUUID,
    author_id TIMEUUID,
    content TEXT,
    edited BOOLEAN,
    PRIMARY KEY ((server_id, channel_id), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC);
