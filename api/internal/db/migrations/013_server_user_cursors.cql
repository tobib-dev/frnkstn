CREATE TABLE IF NOT EXISTS frnkstn.server_user_cursors (
    user_id TIMEUUID,
    server_id TIMEUUID,
    cursor TIMESTAMP,
    join_code TEXT,
    PRIMARY KEY ((user_id), server_id)
);
