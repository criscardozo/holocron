-- This HTPC migrated from Plex to Jellyfin, so the inventory columns no longer
-- describe what fills them. Renaming keeps the rows: the paths, titles and
-- subtitle flags are still valid, only their source changed.
--
-- The item id is not carried over. A Plex ratingKey means nothing to Jellyfin,
-- and a stale value would look like a usable reference; the next sync fills it
-- with the Jellyfin id.
ALTER TABLE media_items RENAME COLUMN plex_guid TO server_item_id;
ALTER TABLE media_items RENAME COLUMN plex_alt_ids TO provider_ids;

UPDATE media_items SET server_item_id = NULL;

-- Plex settings are dropped rather than translated: a Plex token and device id
-- are useless to Jellyfin, and leaving them would keep the old integration
-- looking configured.
DELETE FROM settings WHERE key IN ('plex.url', 'plex.token', 'plex.client_id');
