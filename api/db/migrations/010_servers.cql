CREATE TABLE IF NOT EXISTS frnkstn.servers(
    server_id TIMEUUID PRIMARY KEY,
    server_name TEXT,
    server_bio TEXT,
    owner TIMEUUID,
    deletion_timestamp TIMESTAMP
);

CREATE TABLE IF NOT EXISTS frnkstn.servers_by_user(
    user_id TIMEUUID,
    server_id TIMEUUID,
    server_name TEXT,
    cursor TIMESTAMP,
    deletion_timestamp TIMESTAMP,
    PRIMARY KEY ((user_id), cursor, server_id)
) WITH CLUSTERING ORDER BY (cursor DESC, server_id ASC);

CREATE TABLE IF NOT EXISTS frnkstn.server_moderators(
    server_id TIMEUUID,
    mod_id TIMEUUID,
    mod_name TEXT,
    role TEXT,
    PRIMARY KEY((server_id), mod_id)
) WITH CLUSTERING ORDER BY (mod_id ASC);

CREATE TABLE IF NOT EXISTS frnkstn.server_members(
    server_id TIMEUUID,
    member_id TIMEUUID,
    member_name TEXT,
    role TEXT,
    PRIMARY KEY((server_id), member_id)
) WITH CLUSTERING ORDER BY (member_id ASC);
