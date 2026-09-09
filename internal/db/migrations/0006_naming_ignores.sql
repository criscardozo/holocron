-- Folders the user has said to leave alone.
--
-- A separate table rather than a flag on naming_issues, because every scan
-- deletes and rewrites that table: a flag there would be forgotten the next
-- time anyone pressed refresh, which is exactly when it matters. This outlives
-- the scans.
--
-- Keyed by absolute path. It survives the folder being renamed by hand — the
-- ignore simply stops matching, which is the right outcome: a folder with a
-- different name is a different decision.
CREATE TABLE naming_ignores (
    path       TEXT PRIMARY KEY,
    ignored_at TEXT NOT NULL DEFAULT (datetime('now'))
);
