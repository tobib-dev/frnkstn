CREATE TABLE message_notifications (
    user_id TIMEUUID,
    notification_id TIMEUUID,
    message_id TIMEUUID,
    author_id TIMEUUID,
    author_name TEXT,
    content TEXT,
    seen BOOLEAN,
    PRIMARY KEY ((user_id, seen), notification_id)
) WITH CLUSTERING ORDER BY (notification_id DESC) AND default_time_to_live = 15552000;

CREATE TABLE friend_notifications (
    user_id TIMEUUID,
    notification_id TIMEUUID,
    friendship_id TIMEUUID,
    friend_id TIMEUUID,
    friend_name TEXT,
    status TEXT,
    seen BOOLEAN,
    PRIMARY KEY ((user_id, seen), notification_id)
) WITH CLUSTERING ORDER BY (notification_id DESC) AND default_time_to_live = 15552000;
