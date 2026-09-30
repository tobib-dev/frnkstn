CREATE TABLE frnkstn.friends (
    id TIMEUUID PRIMARY KEY,
    user_id TIMEUUID,
    friend_id TIMEUUID,
    friend_name TEXT,
    status TEXT,
);

CREATE TABLE frnkstn.friends_by_user_id (
    user_id TIMEUUID PRIMARY KEY,
    friend_id TIMEUUID,
    friendship_id TIMEUUID,
    friend_name TEXT,
    status TEXT,
);
