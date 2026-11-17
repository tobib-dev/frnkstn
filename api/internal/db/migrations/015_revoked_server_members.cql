CREATE TABLE IF NOT EXISTS frnkstn.revoked_server_members (
    server_id TIMEUUID,
    member_id TIMEUUID,
    PRIMARY KEY ((server_id), member_id)
);
