# Template Design: Tic-Tac-Toe

## The GeoCities Game Room

Every GeoCities page had a guestbook. The good ones had a game. Tic-tac-toe is the simplest possible multiplayer game — two players, nine squares, no ambiguity. It's the "hello world" of game templates and a proving ground for the full-stack peer model: site UI + database + server-side Lua + cross-peer interaction.

No group protocol needed. No WebSockets. Just the data protocol, a Lua function that validates moves, and a polling loop. Two peers, a 3x3 grid, and the infrastructure that already exists.

---

## How It Works

The host peer installs the tic-tac-toe template. Their site becomes a game lobby. Visitors see open games and can start a challenge. All game state lives in the host's database. Moves are validated server-side by a Lua function — neither player can cheat.

```
Host peer (PeerA)                          Visitor (PeerB)
    |                                           |
    |  installs tic-tac-toe template            |
    |  site shows: "Challenge me!"              |
    |                                           |
    |           PeerB visits /p/<peerA>/         |
    |  <--------------------------------------- |
    |                                           |
    |  site loads, PeerB sees lobby             |
    |  PeerB clicks "New Game"                  |
    |                                           |
    |    lua-call: new_game({})                  |
    |  <--------------------------------------- |
    |    Lua creates game row, PeerB is "O"     |
    |    returns: {game_id, board, turn}         |
    |  ---------------------------------------> |
    |                                           |
    |  PeerA opens site, sees pending game      |
    |  PeerA is "X" (host is always X)          |
    |  PeerA clicks cell (1,0)                  |
    |                                           |
    |    lua-call: make_move({game_id, row, col})|
    |  (local call — PeerA is the host)         |
    |    Lua validates: correct turn, cell empty |
    |    Lua updates board, checks win/draw     |
    |    returns: {board, turn, status}          |
    |                                           |
    |           PeerB polls for update           |
    |    lua-call: game_state({game_id})         |
    |  <--------------------------------------- |
    |    returns: {board, turn, status}          |
    |  ---------------------------------------> |
    |                                           |
    |  PeerB sees PeerA's move, makes their own |
    |  ... (alternating turns until win/draw)    |
```

### Why Lua Validation Matters

Without server-side validation, a malicious visitor could:
- Play out of turn
- Overwrite an occupied cell
- Claim a win that didn't happen
- Modify their opponent's moves

The Lua `make_move` function prevents all of this. It's the referee. The board state in the database is the single source of truth, and only the Lua function can modify it.

---

## Database Schema

```sql
-- Active and completed games
CREATE TABLE games (
    _id          INTEGER PRIMARY KEY AUTOINCREMENT,
    _owner       TEXT NOT NULL,              -- host peer ID (always X)
    _owner_email TEXT DEFAULT '',
    _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    challenger   TEXT NOT NULL DEFAULT '',    -- visitor peer ID (always O)
    challenger_label TEXT DEFAULT '',         -- visitor's display name
    board        TEXT NOT NULL DEFAULT '---------',  -- 9 chars: X, O, or -
    turn         TEXT NOT NULL DEFAULT 'X',  -- whose turn: X or O
    status       TEXT NOT NULL DEFAULT 'waiting',
                 -- waiting | playing | won_x | won_o | draw
    winner       TEXT DEFAULT ''             -- peer ID of winner, or empty
);
```

### Board Encoding

The board is a 9-character string. Each character is `X`, `O`, or `-` (empty). Positions map left-to-right, top-to-bottom:

```
 0 | 1 | 2
-----------
 3 | 4 | 5
-----------
 6 | 7 | 8
```

So `"X--OX---O"` represents:

```
 X |   |
-----------
 O | X |
-----------
   |   | O
```

Why a string instead of a table? It's atomic — one column, one UPDATE, no race conditions. Lua can index into it with `string.sub()` and rebuild it with concatenation. SQLite stores it efficiently. The frontend parses it trivially with `board.charAt(i)` or `board[i]`.

### Insert Policies

```json
{
    "tables": {
        "games": { "insert_policy": "open" }
    }
}
```

The `games` table uses `open` insert policy because visitors need to create games (challenge the host). The Lua function handles all validation — the raw insert policy just needs to allow the row creation.

---

## Lua Functions

Two data functions in `lua/functions/`:

### `move.lua` — Make a Move

The core game logic. Validates the move, updates the board, checks for win/draw, and returns the new state.

```lua
--- Make a move in a tic-tac-toe game
function call(request)
    local p = request.params
    local game_id = p.game_id
    local pos = tonumber(p.position)  -- 0-8

    if not game_id or not pos then
        error("game_id and position required")
    end
    if pos < 0 or pos > 8 then
        error("position must be 0-8")
    end

    -- Load the game
    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, board, turn, status FROM games WHERE _id = ?",
        game_id
    )
    if not rows or #rows == 0 then
        error("game not found")
    end
    local game = rows[1]

    -- Game must be in progress
    if game.status ~= "playing" then
        return { error = "game is not in progress", status = game.status }
    end

    -- Determine which symbol the caller plays
    local symbol
    if goop.peer.id == game._owner then
        symbol = "X"
    elseif goop.peer.id == game.challenger then
        symbol = "O"
    else
        return { error = "you are not a player in this game" }
    end

    -- Check it's the caller's turn
    if game.turn ~= symbol then
        return { error = "not your turn" }
    end

    -- Check cell is empty
    local idx = pos + 1  -- Lua is 1-indexed
    local current = string.sub(game.board, idx, idx)
    if current ~= "-" then
        return { error = "cell is already occupied" }
    end

    -- Place the move
    local new_board = string.sub(game.board, 1, idx - 1)
                   .. symbol
                   .. string.sub(game.board, idx + 1)

    -- Check for winner
    local winner = check_winner(new_board)
    local new_status = "playing"
    local new_turn = (symbol == "X") and "O" or "X"
    local winner_id = ""

    if winner then
        new_status = "won_" .. string.lower(winner)
        new_turn = ""
        if winner == "X" then
            winner_id = game._owner
        else
            winner_id = game.challenger
        end
    elseif not string.find(new_board, "-") then
        -- No empty cells and no winner = draw
        new_status = "draw"
        new_turn = ""
    end

    -- Persist
    goop.db.exec(
        "UPDATE games SET board = ?, turn = ?, status = ?, winner = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        new_board, new_turn, new_status, winner_id, game_id
    )

    return {
        board = new_board,
        turn = new_turn,
        status = new_status,
        winner = winner_id,
        your_symbol = symbol
    }
end

-- Check all winning lines
function check_winner(b)
    local lines = {
        {1,2,3}, {4,5,6}, {7,8,9},  -- rows
        {1,4,7}, {2,5,8}, {3,6,9},  -- cols
        {1,5,9}, {3,5,7}            -- diagonals
    }
    for _, line in ipairs(lines) do
        local a = string.sub(b, line[1], line[1])
        local b2 = string.sub(b, line[2], line[2])
        local c = string.sub(b, line[3], line[3])
        if a ~= "-" and a == b2 and b2 == c then
            return a
        end
    end
    return nil
end

function handle(args)
    return "Visit my site to play tic-tac-toe!"
end
```

### `ttt.lua` — Game Management

Handles game creation, state queries, and lobby listing. One function, multiple operations dispatched via a `params.action` field.

```lua
--- Tic-tac-toe game management
function call(request)
    local action = request.params.action

    if action == "new" then
        return new_game()
    elseif action == "state" then
        return game_state(request.params.game_id)
    elseif action == "lobby" then
        return lobby()
    elseif action == "cancel" then
        return cancel_game(request.params.game_id)
    else
        error("unknown action: " .. tostring(action))
    end
end

function new_game()
    -- Check if this challenger already has an active game
    local existing = goop.db.query(
        "SELECT _id FROM games WHERE challenger = ? AND status IN ('waiting', 'playing')",
        goop.peer.id
    )
    if existing and #existing > 0 then
        return { error = "you already have an active game", game_id = existing[1]._id }
    end

    -- Create the game
    goop.db.exec(
        "INSERT INTO games (_owner, challenger, challenger_label, status) VALUES (?, ?, ?, 'waiting')",
        goop.self.id, goop.peer.id, goop.peer.label
    )

    -- Get the created game ID
    local id = goop.db.scalar("SELECT MAX(_id) FROM games WHERE challenger = ?", goop.peer.id)

    return { game_id = id, your_symbol = "O", status = "waiting" }
end

function game_state(game_id)
    if not game_id then
        error("game_id required")
    end

    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, challenger_label, board, turn, status, winner, _created_at, _updated_at FROM games WHERE _id = ?",
        game_id
    )
    if not rows or #rows == 0 then
        error("game not found")
    end
    local g = rows[1]

    -- Determine caller's symbol
    local your_symbol = ""
    if goop.peer.id == g._owner then
        your_symbol = "X"
    elseif goop.peer.id == g.challenger then
        your_symbol = "O"
    end

    return {
        game_id = g._id,
        board = g.board,
        turn = g.turn,
        status = g.status,
        winner = g.winner,
        your_symbol = your_symbol,
        challenger_label = g.challenger_label,
        created_at = g._created_at
    }
end

function lobby()
    -- Active games (most recent first)
    local games = goop.db.query(
        "SELECT _id, challenger, challenger_label, board, turn, status, winner, _created_at FROM games ORDER BY _id DESC LIMIT 20"
    )
    return { games = games or {} }
end

function cancel_game(game_id)
    if not game_id then
        error("game_id required")
    end

    -- Only players can cancel, and only waiting/playing games
    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, status FROM games WHERE _id = ?",
        game_id
    )
    if not rows or #rows == 0 then
        error("game not found")
    end
    local g = rows[1]

    if goop.peer.id ~= g._owner and goop.peer.id ~= g.challenger then
        return { error = "you are not a player in this game" }
    end
    if g.status ~= "waiting" and g.status ~= "playing" then
        return { error = "game is already finished" }
    end

    goop.db.exec(
        "UPDATE games SET status = 'draw', _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        game_id
    )

    return { status = "cancelled" }
end

function handle(args)
    return "Visit my site to play tic-tac-toe!"
end
```

---

## Frontend

### Views

The frontend has three states based on context:

| Context | What's shown |
|---|---|
| **Owner, no active game** | Lobby: list of recent/pending games, waiting for challengers |
| **Owner, active game** | Game board (playing as X) |
| **Visitor, no active game** | "Challenge [host]!" button + recent game history |
| **Visitor, active game** | Game board (playing as O) |
| **Spectator** | Read-only board view (if they aren't a player in the game) |

### Owner View (Lobby)

```
    ╔══════════════════════════════════════╗
    ║        Tic-Tac-Toe                   ║
    ║        Challenge me!                 ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║  Pending Challenges                  ║
    ║  ┌──────────────────────────────┐    ║
    ║  │ Alice wants to play     [▶]  │    ║
    ║  └──────────────────────────────┘    ║
    ║                                      ║
    ║  Recent Games                        ║
    ║  ┌──────────────────────────────┐    ║
    ║  │ vs Alice    ✕ won    2m ago  │    ║
    ║  │ vs Bob      draw     1h ago  │    ║
    ║  │ vs Charlie  ○ won    3h ago  │    ║
    ║  └──────────────────────────────┘    ║
    ║                                      ║
    ╚══════════════════════════════════════╝
```

When the owner clicks [▶] on a pending challenge, the game status changes from `waiting` to `playing` and the board view loads. The host plays as X and goes first.

### Visitor View (Challenge)

```
    ╔══════════════════════════════════════╗
    ║        Tic-Tac-Toe                   ║
    ║        [Host]'s game room            ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║     ┌──────────────────────┐         ║
    ║     │   Challenge [Host]!  │         ║
    ║     └──────────────────────┘         ║
    ║                                      ║
    ║  Your record: 2 wins, 1 loss         ║
    ║                                      ║
    ║  Recent Games                        ║
    ║  ┌──────────────────────────────┐    ║
    ║  │ Alice vs Host   ✕ won  2m   │    ║
    ║  │ Bob vs Host     draw   1h   │    ║
    ║  └──────────────────────────────┘    ║
    ║                                      ║
    ╚══════════════════════════════════════╝
```

Clicking "Challenge" calls `ttt.lua` with `action: "new"`, which creates a game in `waiting` status. The visitor then sees a "Waiting for [host] to accept..." screen, polling `game_state` until the status changes to `playing`.

### Game Board

```
    ╔══════════════════════════════════════╗
    ║  You: ✕   vs   Alice: ○             ║
    ║  Your turn                           ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║         │         │                  ║
    ║    ✕    │         │    ○             ║
    ║         │         │                  ║
    ║  ───────┼─────────┼───────           ║
    ║         │         │                  ║
    ║         │    ✕    │                  ║
    ║         │         │                  ║
    ║  ───────┼─────────┼───────           ║
    ║         │         │                  ║
    ║    ○    │         │                  ║
    ║         │         │                  ║
    ║                                      ║
    ╚══════════════════════════════════════╝
```

- Empty cells are clickable on your turn, disabled on opponent's turn
- After clicking, the move is sent to `move.lua` for validation
- If valid, the board updates immediately (optimistic) and polling continues for the opponent's move
- On game end (win/draw), the result screen shows with a "Play Again" option

### Polling

Without the group protocol, the frontend polls for updates:

```javascript
// Poll every 2 seconds while it's the opponent's turn
var pollTimer = null;

function startPolling(gameId) {
    stopPolling();
    pollTimer = setInterval(async function () {
        var state = await db.call("ttt", { action: "state", game_id: gameId });
        renderBoard(state);
        if (state.status !== "playing") {
            stopPolling();
        }
    }, 2000);
}

function stopPolling() {
    if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
    }
}
```

2-second polling is a pragmatic choice. Tic-tac-toe moves take seconds to think about — a 2-second delay between seeing the opponent's move is acceptable. The Lua function call is lightweight (one SELECT query), so even at 0.5 req/s per player, the load on the host peer is negligible.

When the group protocol is implemented, polling can be replaced with push notifications. But polling works now, with zero new infrastructure.

---

## File Structure

```
internal/sitetemplates/tictactoe/
    manifest.json
    schema.sql
    index.html
    css/
        style.css
    js/
        app.js
    lua/
        functions/
            move.lua        -- move validation + board update
            ttt.lua         -- game management (new, state, lobby, cancel)
    images/
        .keep
```

### manifest.json

```json
{
    "name": "Tic-Tac-Toe",
    "description": "Classic tic-tac-toe — challenge visitors to a game on your site.",
    "category": "games",
    "icon": "❌⭕",
    "tables": {
        "games": { "insert_policy": "open" }
    }
}
```

### schema.sql

```sql
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
    winner            TEXT DEFAULT ''
);
```

---

## Game Flow: Step by Step

### 1. Template Install

Owner applies the tic-tac-toe template. The `games` table is created. Two Lua functions (`move.lua`, `ttt.lua`) are written to `site/lua/functions/`. The engine's fsnotify watcher picks them up and compiles them.

### 2. Visitor Arrives

Visitor loads `/p/<host>/`. The JS detects remote context (URL contains `/p/<peerID>/`). All `Goop.data` calls will route to the host's database via the data protocol proxy.

### 3. Visitor Challenges

```javascript
var result = await db.call("ttt", { action: "new" });
// result = { game_id: 1, your_symbol: "O", status: "waiting" }
```

The `ttt.lua` function:
1. Checks the visitor doesn't already have an active game
2. Inserts a row: `_owner` = host peer ID (always X), `challenger` = visitor peer ID
3. Returns the game ID

### 4. Host Accepts

Host opens their own site, sees the pending challenge in the lobby. Clicks "Accept." The JS calls:

```javascript
await db.call("move", { game_id: 1, position: 4 });
// Host plays X in the center
```

Wait — the game is still in `waiting` status. The accept action should transition it to `playing` first. Two approaches:

**Option A**: The host's first move implicitly starts the game. The `move.lua` function checks if status is `waiting` and the caller is the host (`_owner`), and transitions to `playing` before processing the move.

**Option B**: Add an explicit `accept` action to `ttt.lua` that transitions the game to `playing`. Then the host makes their first move separately.

**Recommendation: Option A.** It's one fewer round-trip and feels more natural — the host sees the challenge, clicks a cell, and the game begins. The `move.lua` function adds three lines:

```lua
-- Auto-start: host's first move begins the game
if game.status == "waiting" and symbol == "X" then
    game.status = "playing"
end
```

### 5. Turns Alternate

Each player polls `game_state` on a 2-second interval. When it's their turn, empty cells are clickable. On click:

```javascript
var result = await db.call("move", { game_id: gameId, position: cellIndex });
if (result.error) {
    showError(result.error);
} else {
    renderBoard(result);
    if (result.status === "playing") {
        startPolling(gameId);  // wait for opponent
    }
}
```

### 6. Game Ends

The `move.lua` function checks for a winner after every move. When three in a row are found (or no empty cells remain):

- `status` becomes `won_x`, `won_o`, or `draw`
- `winner` is set to the winning peer's ID (empty on draw)
- `turn` is cleared

The frontend shows the result:

```
    ╔══════════════════════════════════════╗
    ║                                      ║
    ║         You won! 🎉                  ║
    ║                                      ║
    ║    ✕ │ ○ │ ✕                         ║
    ║   ───┼───┼───                        ║
    ║    ○ │ ✕ │                            ║
    ║   ───┼───┼───                        ║
    ║      │   │ ✕    ← winning line       ║
    ║                                      ║
    ║     [Play Again]  [Back to Lobby]    ║
    ║                                      ║
    ╚══════════════════════════════════════╝
```

### 7. Rematch

"Play Again" calls `ttt.lua` with `action: "new"` — creates a fresh game. The old game stays in the database as history.

---

## Styling

The board should feel tactile and fun. Not a flat grid with letters — something with character.

```css
.ttt-board {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 4px;
    max-width: 300px;
    margin: 2rem auto;
    aspect-ratio: 1;
}

.ttt-cell {
    background: #f8f7f4;
    border: 2px solid #e2ddd5;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2.5rem;
    cursor: pointer;
    transition: background 0.15s, transform 0.1s;
    user-select: none;
}

.ttt-cell:hover:not(.taken):not(.disabled) {
    background: #eee8df;
    transform: scale(1.05);
}

.ttt-cell.taken {
    cursor: default;
}

.ttt-cell.disabled {
    cursor: not-allowed;
    opacity: 0.7;
}

.ttt-cell.x { color: #2563eb; }  /* blue for X */
.ttt-cell.o { color: #dc2626; }  /* red for O */

.ttt-cell.win {
    background: #d1fae5;
    border-color: #6ee7b7;
}
```

X and O rendered as styled Unicode or SVG — not plain letters. Something like:

- X: bold sans-serif `✕` or a hand-drawn cross SVG
- O: `○` with a slight stroke or a ring SVG

The winning three cells get a highlight (green background) so the line is immediately visible.

---

## Edge Cases

| Case | Handling |
|---|---|
| Visitor challenges but host never accepts | Game stays in `waiting`. Visitor can cancel. Lobby shows "waiting" with timestamp so host can see stale challenges. |
| Player disconnects mid-game | Game stays in `playing` in the database. When they reconnect and revisit, `game_state` picks up where they left off. No state is lost. |
| Host goes offline | Visitor can't reach the site at all — this is inherent to the ephemeral web. The game resumes when the host comes back online. |
| Player tries to move twice | `move.lua` checks `game.turn != symbol` and returns an error. |
| Visitor opens game in two tabs | Each tab polls independently. Moves are atomic (single UPDATE with WHERE clause). No corruption possible. |
| Multiple visitors challenge simultaneously | Each gets their own game row. The host sees all pending challenges and picks which to play. |
| Someone who isn't a player calls move | `move.lua` checks `goop.peer.id` against `_owner` and `challenger`. Returns error for non-players. |

---

## Win/Loss Tracking (Optional Enhancement)

A `stats` view on the lobby page showing the host's record:

```lua
-- In ttt.lua, add a "stats" action
function stats()
    local wins = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE winner = ? AND status LIKE 'won_%'",
        goop.self.id
    ) or 0
    local losses = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE winner != '' AND winner != ? AND status LIKE 'won_%'",
        goop.self.id
    ) or 0
    local draws = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE status = 'draw'"
    ) or 0

    return { wins = wins, losses = losses, draws = draws }
end
```

Displayed on the lobby:

```
Your record: 5W - 2L - 1D
```

Per-visitor record could also be computed by filtering on `challenger`:

```lua
local vs_wins = goop.db.scalar(
    "SELECT COUNT(*) FROM games WHERE challenger = ? AND winner = ?",
    goop.peer.id, goop.self.id
) or 0
```

---

## Future: Group Protocol Upgrade

When the group protocol (`/goop/group/1.0.0`) is implemented, tic-tac-toe becomes real-time:

1. When a game starts, both players join a group
2. Moves are sent via the group channel — instant delivery, no polling
3. The Lua function still validates (the group message triggers a `lua-call`), but the result is pushed to both players immediately
4. Spectators can join the group in read-only mode and watch live

The upgrade is additive. The polling-based version keeps working. The group protocol just replaces `setInterval` with event-driven updates:

```javascript
// Before (polling):
setInterval(function () { checkState(gameId); }, 2000);

// After (group protocol):
Goop.group.join("ttt-" + gameId, function (msg) {
    if (msg.type === "move") renderBoard(msg.payload);
});
```

The database schema, Lua functions, and game logic don't change at all. Only the transport layer upgrades.

---

## Why This Template Matters

Tic-tac-toe is trivial as a game. As a template, it's a blueprint:

- **Server-side validation** — the Lua function is the referee, preventing cheating
- **Turn-based multiplayer** — two peers interact through a shared database
- **Polling pattern** — works without real-time infrastructure, upgradeable later
- **Lobby/matchmaking** — visitors discover and join games
- **Game history** — past games persist in the database
- **Full-stack template** — HTML + CSS + JS + schema + Lua, all wired together

Every pattern here applies to more complex games: chess, checkers, Connect Four, Battleship, card games. The board encoding changes, the validation logic grows, but the architecture is identical:

1. Frontend renders the board
2. Player clicks → `lua-call` validates the move
3. Database is the source of truth
4. Opponent polls (or receives push) for updates
5. Lua checks for game-over conditions

Build tic-tac-toe, and you've built the framework for any turn-based game on the ephemeral web.
