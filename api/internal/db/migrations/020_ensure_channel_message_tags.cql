ALTER TABLE frnkstn.channel_messages
ADD tags FROZEN<SET<TIMEUUID>>;
