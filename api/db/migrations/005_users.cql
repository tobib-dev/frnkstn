DROP TABLE IF EXISTS frnkstn.users_by_github_id;

CREATE TABLE frnkstn.users_by_github_id (
    github_id bigint PRIMARY KEY,
    user_id timeuuid,
    name text,
    username text
);
