CREATE TABLE questions (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  question     TEXT NOT NULL,
  option_a     TEXT NOT NULL,
  option_b     TEXT NOT NULL,
  option_c     TEXT NOT NULL,
  option_d     TEXT NOT NULL,
  correct      TEXT NOT NULL DEFAULT 'a'
);

CREATE TABLE scores (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  score        INTEGER DEFAULT 0,
  total        INTEGER DEFAULT 0,
  peer_label   TEXT DEFAULT ''
);
