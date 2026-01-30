CREATE TABLE photos (
  _id         INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner      TEXT NOT NULL,
  _created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  filename    TEXT NOT NULL,
  caption     TEXT DEFAULT ''
);
