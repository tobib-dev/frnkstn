CREATE TABLE IF NOT EXISTS frnkstn.refresh_tokens (
    refresh_token TEXT PRIMARY KEY,
    user_id TIMEUUID,
    expiration TIMESTAMP,
    revoked_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS frnkstn.refresh_tokens_by_github_id (
    github_id BIGINT PRIMARY KEY,
    user_id TIMEUUID,
    refresh_token TEXT,
    expiration TIMESTAMP,
    revoked_at TIMESTAMP
);
