ALTER TABLE frnkstn.server_channel_threads ADD tags FROZEN<SET<TIMEUUID>>;
ALTER TABLE frnkstn.threads_by_channel ADD tags FROZEN<SET<TIMEUUID>>;
