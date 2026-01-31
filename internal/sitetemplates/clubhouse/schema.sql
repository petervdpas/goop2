CREATE TABLE rooms (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  name         TEXT NOT NULL,
  description  TEXT DEFAULT '',
  group_id     TEXT NOT NULL,
  max_members  INTEGER DEFAULT 0,
  status       TEXT DEFAULT 'open'
);
