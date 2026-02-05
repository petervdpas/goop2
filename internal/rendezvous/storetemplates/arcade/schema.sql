-- Arcade game schema

-- High scores / leaderboard
CREATE TABLE high_scores (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _owner_email TEXT DEFAULT '',
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  player_label TEXT DEFAULT '',
  score        INTEGER NOT NULL,
  level        INTEGER DEFAULT 1,
  time_ms      INTEGER DEFAULT 0
);

-- User-created levels (owner can add custom levels)
CREATE TABLE levels (
  _id          INTEGER PRIMARY KEY AUTOINCREMENT,
  _owner       TEXT NOT NULL,
  _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  level_num    INTEGER NOT NULL,
  title        TEXT DEFAULT '',
  data         TEXT NOT NULL
);

-- Seed demo level
INSERT INTO levels (_owner, level_num, title, data) VALUES (
  '__system__', 1, 'Demo Level',
  '{"width":20,"height":15,"tiles":[[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,0,0,1,1,1,0,0,0,0,1,1,1,0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[0,0,0,1,1,0,0,0,0,0,0,0,0,0,0,1,1,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1],[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1]],"spawn":{"x":32,"y":192},"coins":[{"x":96,"y":128},{"x":208,"y":128},{"x":152,"y":64}]}'
);
