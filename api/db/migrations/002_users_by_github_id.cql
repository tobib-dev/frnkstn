CREATE TABLE IF NOT EXISTS frnkstn.users_by_github_id (
    github_id bigint PRIMARY KEY,
    user_id uuid,
    name text,
    username text
);
