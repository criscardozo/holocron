-- Store the alternate identifiers (imdb/tmdb/tvdb) Plex returns, as JSON, so
-- the .nfo generator can include them without re-querying Plex.
ALTER TABLE media_items ADD COLUMN plex_alt_ids TEXT NOT NULL DEFAULT '{}';
