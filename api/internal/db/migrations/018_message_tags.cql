-- Add column tags for user's tagged in messages
-- tags is a frozen set. Hence it needs to be
-- completely updated on every message update
ALTER TABLE frnkstn.channel_messages
ADD tags FROZEN<SET<TIMEUUID>>;
