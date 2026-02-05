CREATE TABLE columns (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  name         TEXT NOT NULL,
  position     INTEGER NOT NULL DEFAULT 0,
  color        TEXT DEFAULT '#5b6abf'
);

CREATE TABLE cards (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  column_id    INTEGER NOT NULL,
  title        TEXT NOT NULL,
  description  TEXT DEFAULT '',
  position     INTEGER NOT NULL DEFAULT 0,
  color        TEXT DEFAULT '',
  assignee     TEXT DEFAULT '',
  due_date     TEXT DEFAULT ''
);

-- Seed default columns
INSERT INTO columns (_owner, name, position, color) VALUES ('__system__', 'To Do', 0, '#6366f1');
INSERT INTO columns (_owner, name, position, color) VALUES ('__system__', 'In Progress', 1, '#f59e0b');
INSERT INTO columns (_owner, name, position, color) VALUES ('__system__', 'Done', 2, '#22c55e');
