CREATE TABLE IF NOT EXISTS frnkstn.users (
    id TIMEUUID PRIMARY KEY,
    username TEXT,
    github_id BIGINT,
    name TEXT
);

CREATE TABLE IF NOT EXISTS frnkstn.users_by_username (
    username TEXT PRIMARY KEY,
    user_id TIMEUUID
);
