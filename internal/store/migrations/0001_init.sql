-- Schema 1: the snapshot graph and the working sets it covers.
CREATE TABLE snapshots (
  id          TEXT PRIMARY KEY,   -- ULID
  parent_id   TEXT REFERENCES snapshots(id),
  label       TEXT,
  created_at  INTEGER NOT NULL,   -- unix epoch
  workset_id  TEXT NOT NULL REFERENCES worksets(id),
  fs_handle   TEXT NOT NULL,      -- backend path or shadow copy ID
  backend     TEXT NOT NULL,      -- 'btrfs' | 'apfs' | 'vss' | 'copy'
  container   TEXT,               -- container checkpoint ID, nullable
  auto        INTEGER NOT NULL    -- 1 if created implicitly before an action
);

CREATE TABLE worksets (
  id        TEXT PRIMARY KEY,
  name      TEXT NOT NULL,
  paths     TEXT NOT NULL,          -- JSON array of absolute paths
  container INTEGER NOT NULL DEFAULT 0  -- 1 enables the container layer
);

CREATE UNIQUE INDEX idx_worksets_name ON worksets(name);
CREATE INDEX idx_snapshots_parent ON snapshots(parent_id);
CREATE INDEX idx_snapshots_workset ON snapshots(workset_id, created_at);
