-- Holocron no longer writes .nfo files: Jellyfin writes them itself when a
-- library has "save metadata as NFO" on, and two writers over one path means
-- the last one wins. The column recorded when Holocron last wrote one, which
-- from here on is never.
ALTER TABLE media_items DROP COLUMN nfo_written_at;
