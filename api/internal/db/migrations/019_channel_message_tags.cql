-- Ensure existing deployments that recorded migration 018 before the tags
-- column was available can store message tags. Non-primary-key columns are
-- nullable in CQL, so messages without tags retain a null value.
ALTER TABLE frnkstn.channel_messages
ADD IF NOT EXISTS tags FROZEN<SET<TIMEUUID>>;
