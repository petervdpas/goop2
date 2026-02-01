CREATE TABLE games (
  _id              INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner           TEXT NOT NULL,
  _owner_email     TEXT DEFAULT '',
  _created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
  _updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
  challenger        TEXT NOT NULL DEFAULT '',
  challenger_label  TEXT DEFAULT '',
  board             TEXT NOT NULL DEFAULT '---------',
  turn              TEXT NOT NULL DEFAULT 'X',
  status            TEXT NOT NULL DEFAULT 'waiting',
  winner            TEXT DEFAULT '',
  mode              TEXT NOT NULL DEFAULT 'pvp'
);
