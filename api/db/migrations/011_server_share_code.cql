CREATE TABLE IF NOT EXISTS frnkstn.server_share_code(
    share_code TEXT PRIMARY KEY,
    sender TIMEUUID,
    receiver TIMEUUID,
    server_id TIMEUUID,
    used BOOLEAN
) WITH default_time_to_live = 2592000;
