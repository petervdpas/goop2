CREATE TABLE notes (
  _id         INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner      TEXT NOT NULL,
  _created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  title       TEXT NOT NULL,
  description TEXT,
  category    TEXT DEFAULT 'general',
  contact     TEXT,
  color       TEXT DEFAULT 'yellow'
);
