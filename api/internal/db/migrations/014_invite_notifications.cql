CREATE TABLE IF NOT EXISTS frnkstn.invite_notifications (
    user_id TIMEUUID,
    notification_id TIMEUUID,
    server_id TIMEUUID,
    server_name TEXT,
    sender_id TIMEUUID,
    share_code TEXT,
    PRIMARY KEY ((user_id), notification_id)
) WITH CLUSTERING ORDER BY (notification_id DESC)
AND default_time_to_live = 2592000;
