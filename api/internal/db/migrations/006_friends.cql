CREATE TABLE frnkstn.friends (
    id TIMEUUID PRIMARY KEY,
    user_id TIMEUUID,
    friend_id TIMEUUID,
    friend_name TEXT,
    status TEXT,
);

CREATE TABLE frnkstn.friends_by_user_id (
    user_id TIMEUUID,
    id TIMEUUID,
    friend_id TIMEUUID,
    friend_name TEXT,
    status TEXT,
    PRIMARY KEY (user_id, id),
) WITH CLUSTERING ORDER BY (id ASC);
