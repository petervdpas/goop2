CREATE TABLE posts (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  title        TEXT NOT NULL,
  body         TEXT NOT NULL,
  author_name  TEXT DEFAULT '',
  image        TEXT DEFAULT '',
  slug         TEXT,
  published    INTEGER DEFAULT 1
);

CREATE TABLE blog_config (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  key          TEXT NOT NULL,
  value        TEXT NOT NULL DEFAULT ''
);
