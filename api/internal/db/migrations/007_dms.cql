CREATE TABLE direct_messages (
	id TIMEUUID,
	message_id TIMEUUID,
	author_id TIMEUUID,
	receiver_id TIMEUUID,
	content TEXT,
	PRIMARY KEY ((id), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC);

CREATE TABLE direct_messages_by_user (
	user_id TIMEUUID,
	last_message_time TIMESTAMP,
	id TIMEUUID,
	PRIMARY KEY ((user_id), last_message_time, id)
) WITH CLUSTERING ORDER BY (last_message_time DESC, id DESC);

CREATE table direct_messages_by_message (
	message_id TIMEUUID,
	id TIMEUUID,
	author_id TIMEUUID,
	receiver_id TIMEUUID,
	content TEXT,
	PRIMARY KEY ((message_id), id)
) WITH CLUSTERING ORDER BY (id DESC);
