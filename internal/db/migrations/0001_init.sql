-- Initial schema for Holocron.
-- Tables grow per phase; this migration covers the foundations plus the tables
-- the early features (disk usage, naming, media) will populate.

-- Key/value application settings edited from the UI (media paths, external
-- service credentials, API keys). Values may be secret: the DB file is
-- owner-only on disk.
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Folders the user chooses to watch (disk-usage widget and naming validator).
CREATE TABLE watched_folders (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    label      TEXT NOT NULL,
    path       TEXT NOT NULL UNIQUE,
    purpose    TEXT NOT NULL DEFAULT 'disk', -- disk | movies | tv
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Cache of the most recent disk scan per watched folder (result stored as JSON).
CREATE TABLE scan_results (
    folder_id  INTEGER PRIMARY KEY REFERENCES watched_folders(id) ON DELETE CASCADE,
    result     TEXT NOT NULL,
    scanned_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Folders that violate the "Title (Year)" naming convention.
CREATE TABLE naming_issues (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    path      TEXT NOT NULL,
    type      TEXT NOT NULL,          -- movies | tv
    expected  TEXT NOT NULL,
    found     TEXT NOT NULL,
    resolved  INTEGER NOT NULL DEFAULT 0,
    detected_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Inventory of detected media (for .nfo generation and subtitle checks).
CREATE TABLE media_items (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    path           TEXT NOT NULL UNIQUE,
    type           TEXT NOT NULL,     -- movie | show
    title          TEXT,
    year           INTEGER,
    plex_guid      TEXT,
    has_subs_es    INTEGER NOT NULL DEFAULT 0,
    nfo_written_at TEXT
);

-- Background jobs (disk-scan, nfo-generate, subtitle-search, ...).
CREATE TABLE jobs (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    status      TEXT NOT NULL,        -- running | done | error
    progress    INTEGER NOT NULL DEFAULT 0,
    error       TEXT,
    result      TEXT,
    started_at  TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT
);
