-- Add column tags for user's tagged in messages
-- tags is a frozen set. Hence it needs to be
-- completely updated on every message update
ALTER TABLE frnkstn.channel_messages
ADD tags FROZEN<SET<TIMEUUID>>;

CREATE TABLE server_message_notifications(
    user_id TIMEUUID,
    notification_id TIMEUUID,
    server_id TIMEUUID,
    channel_id TIMEUUID,
    message_id TIMEUUID,
    content TEXT,
    seen BOOLEAN,
    PRIMARY KEY ((user_id, seen), notification_id)
) WITH CLUSTERING ORDER BY (notification_id DESC);
