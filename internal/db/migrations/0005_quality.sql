-- The library quality report: one cached document, replaced by each scan.
-- Kept out of media_items because it covers episodes too — the inventory is
-- movies and series only, and 2000 episode rows would change what it means.
-- The CHECK is what keeps it a single row instead of a growing history: only
-- the latest report is ever shown, and an old one would be misleading.
CREATE TABLE quality_reports (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    generated_at TEXT NOT NULL DEFAULT (datetime('now')),
    report       TEXT NOT NULL
);
