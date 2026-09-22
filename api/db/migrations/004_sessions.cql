CREATE TABLE IF NOT EXISTS frnkstn.sessions (
    id TIMEUUID PRIMARY KEY,
    access_token TEXT,
    expires_in TIMESTAMP,
    refresh_token TEXT,
    refresh_token_expires_in TIMESTAMP,
    user_id TIMEUUID,
    signed_out_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS frnkstn.sessions_by_user_id (
    user_id TIMEUUID PRIMARY KEY,
    session_id TIMEUUID,
    refresh_token TEXT,
    expires_in TIMESTAMP,
    signed_out_at TIMESTAMP
);
