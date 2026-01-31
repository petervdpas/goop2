CREATE TABLE responses (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  q1           TEXT DEFAULT '',
  q2           TEXT DEFAULT '',
  q3           TEXT DEFAULT ''
);
