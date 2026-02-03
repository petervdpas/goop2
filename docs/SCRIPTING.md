# Goop² Lua Scripting

## Overview

Lua scripting turns every Goop2 peer into a programmable node. It has two faces:

**Phase 1 — Chat commands.** When a remote peer sends a direct message starting with `!`, the local chat manager dispatches it to a Lua script and replies with the result. Think `!weather London` -> `"London: 12C, partly cloudy"`. This works over the existing `/goop/chat/1.0.0` protocol.

**Phase 2 — Data functions.** Lua scripts become server-side compute for the peer's ephemeral site. A visitor's browser calls a Lua function on the host peer via `/goop/data/1.0.0` and gets structured JSON back. This gives peers backend logic. A quiz site can score answers server-side so visitors can't cheat. A marketplace peer can validate bids. A game host can enforce rules.

Both phases share the same Lua engine, sandbox, and script directory.

### Why Lua

- Tiny, embeddable, battle-tested (30 years of game/plugin scripting)
- Go has mature embedding libraries (`gopher-lua`)
- Sandboxing is straightforward — disable `os`, `io`, `loadfile`, `dofile` at the VM level
- Scripts are readable by non-programmers
- Fast startup — a Lua VM initializes in microseconds

---

## Script Location & Lifecycle

### Where Scripts Live

```bash
{peerDir}/
  site/
    lua/
      weather.lua       -- chat command: !weather
      time.lua          -- chat command: !time
      help.lua          -- chat command: !help
      functions/
        score-quiz.lua  -- data function
        ttt.lua         -- data function
        move.lua        -- data function
    index.html
    style.css
```

Chat scripts live in `site/lua/`. Each `.lua` file is one command. The filename (minus extension) is the command name.

Data functions live in `site/lua/functions/`. Each `.lua` file defines a `call(request)` entry point invocable via the data protocol.

Both directories are **excluded from site serving** — scripts are never served to visitors via `/goop/site/1.0.0`.

### Loading & Reloading

**Initial load**: When the Lua engine starts, scan `site/lua/*.lua` and `site/lua/functions/*.lua`, compile each into a `*lua.FunctionProto`. Store compiled protos in a map keyed by command/function name.

**Hot reload**: Watch with `fsnotify`. On file change, re-compile and replace. On deletion, remove. No restart required.

**Compilation errors**: Log the error, skip the script. The previous working version (if any) remains active. Never crash on a bad script.

---

## Execution Model

### Chat Command Dispatch

```bash
Remote peer                          Local peer
    |                                    |
    |  SendDirect("!weather London")     |
    | ---------------------------------> |
    |                                    | handleStream() receives message
    |                                    | detects "!" prefix
    |                                    | extracts command="weather", args="London"
    |                                    | looks up "weather" in script map
    |                                    | creates fresh Lua VM
    |                                    | injects goop.* API
    |                                    | runs weather.lua with args
    |                                    | captures return value
    |                                    | SendDirect(reply) back to sender
    |  "London: 12C, partly cloudy"      |
    | <--------------------------------- |
```

### Data Function Dispatch

```bash
Visitor browser
    -> goopData.call("score-quiz", params)
    -> local Goop2 viewer
    -> /goop/data/1.0.0 stream to host peer
    -> host peer's data handler sees op="lua-call"
    -> dispatches to Lua engine
    -> Lua script runs with goop.db access
    -> returns structured result
    -> JSON response back through the stream
    -> visitor's browser receives result
```

### One VM Per Invocation

Each script execution gets a **fresh Lua VM**. No state leaks between invocations or between callers. The cost is small — `gopher-lua` creates a new VM in ~50us.

### Timeouts

Every script execution has a hard timeout (default: 5 seconds, configurable). Implemented via a goroutine with `context.WithTimeout`. If exceeded, the VM is forcibly closed (`L.Close()`) using a `sync.Once` guard to prevent double-close races.

### Memory Limits

Memory is controlled at two levels:

1. **Registry size limits** — The Lua VM is configured with `RegistryMaxSize` derived from `MaxMemoryMB` (~48 bytes per slot). This caps table/string allocations within the VM.
2. **Process-level monitoring** — A background goroutine polls `runtime.ReadMemStats` every 100ms. If process memory grows by more than `MaxMemoryMB` from baseline, the VM is killed.

### Concurrency

Multiple peers can invoke scripts simultaneously. Each gets its own VM, so there's no locking beyond the script map's `sync.RWMutex` — reads (execution) are concurrent, writes (reload) are exclusive.

---

## API Reference

Scripts interact with Goop2 through a `goop` table injected into each VM.

### Script Entry Points

**Chat commands** define a `handle` function:

```lua
function handle(args)
    return "You said: " .. args
end
```

**Data functions** define a `call` function:

```lua
--- Score a quiz submission
function call(request)
    local answers = request.params.answers
    -- ... process and return structured result ...
    return { score = 3, total = 5 }
end
```

A script can define both `handle` and `call` — the chat command responds with a text hint, the data function does the real work.

### `goop.peer`

Information about the calling peer.

```lua
goop.peer.id       -- string: caller's peer ID (e.g., "12D3KooW...")
goop.peer.label    -- string: caller's profile label (if known), or ""
```

### `goop.self`

Information about the local peer (script owner).

```lua
goop.self.id       -- string: local peer ID
goop.self.label    -- string: local profile label
```

### `goop.http`

HTTP client for external API calls. Subject to timeout and sandboxing.

```lua
-- GET request
local body, err = goop.http.get(url)

-- POST request (Content-Type is application/x-www-form-urlencoded)
local body, err = goop.http.post(url, payload)
```

- `body`: response body (string), or nil on error
- `err`: error message (string), or nil on success
- Requests inherit the script's timeout
- Only `http://` and `https://` schemes allowed
- Requests to private/loopback addresses are blocked (SSRF protection)

### `goop.json`

JSON encoding/decoding for API responses.

```lua
local tbl = goop.json.decode('{"temp": 12, "city": "London"}')
print(tbl.temp)  -- 12

local str = goop.json.encode({temp = 12, city = "London"})
```

### `goop.kv`

Persistent key-value store scoped per script. Backed by a file in `site/lua/.state/<scriptname>.json`. Survives restarts.

```lua
goop.kv.set("api_key", "abc123")
local key = goop.kv.get("api_key")  -- "abc123"
goop.kv.del("api_key")
```

- Keys and values are strings
- Maximum 1000 keys per script, 64KB total

### `goop.log`

Logging visible in the Goop2 application log.

```lua
goop.log.info("processing request from " .. goop.peer.id)
goop.log.warn("API key missing, using fallback")
goop.log.error("failed to parse response")
```

### `goop.commands()`

Returns a Lua table (array) of all loaded command names. Used by `help.lua`.

### `goop.db` (Data Functions Only)

Database access available only in data functions (not chat commands).

```lua
-- Read query, returns array of row tables
local rows = goop.db.query("SELECT * FROM answers WHERE quiz_id = ?", quiz_id)

-- Single scalar value
local count = goop.db.scalar("SELECT COUNT(*) FROM submissions WHERE peer_id = ?", goop.peer.id)

-- Write operations
goop.db.exec("UPDATE games SET board = ? WHERE _id = ?", new_board, game_id)
```

- `query` allows only SELECT/WITH statements
- `exec` allows only INSERT/UPDATE/DELETE/REPLACE
- Parameterized queries only — no SQL injection
- Row limit: 1000 rows max, 1MB serialized
- `goop.peer.id` available for identity-based filtering

### Data Protocol Request/Response Format

**Request:**

```json
{
    "op": "lua-call",
    "function": "score-quiz",
    "params": { "quiz_id": "midterm-2026", "answers": { "q1": "B" } }
}
```

**Response:**

```json
{ "ok": true, "result": { "score": 2, "total": 3, "passed": false } }
```

**Function discovery:**

```json
{ "op": "lua-list" }
```

Returns all functions that define `call()`, with descriptions from `---` comments.

---

## Example Scripts

### help.lua

```lua
function handle(args)
    local commands = goop.commands()
    local lines = {"Available commands:"}
    for _, cmd in ipairs(commands) do
        table.insert(lines, "  !" .. cmd)
    end
    return table.concat(lines, "\n")
end
```

### weather.lua

```lua
function handle(args)
    if args == "" then return "Usage: !weather <city>" end

    local key = goop.kv.get("api_key")
    if not key then return "Weather API key not configured." end

    local url = "https://api.openweathermap.org/data/2.5/weather"
        .. "?q=" .. args .. "&appid=" .. key .. "&units=metric"

    local body, err = goop.http.get(url)
    if err then return "Error: " .. err end

    local data = goop.json.decode(body)
    return string.format("%s: %s C, %s",
        data.name, tostring(math.floor(data.main.temp)),
        data.weather[1].description)
end
```

### greet.lua

```lua
function handle(args)
    local label = goop.peer.label
    if label ~= "" then
        return "Hello, " .. label .. "!"
    else
        return "Hello, peer " .. string.sub(goop.peer.id, 1, 8) .. "...!"
    end
end
```

---

## Security

### Sandbox

Each Lua VM is created with restricted standard libraries.

**Available:**

- `string` — full string library
- `table` — full table library
- `math` — full math library
- `os.time`, `os.date`, `os.clock` — time functions only
- `tonumber`, `tostring`, `type`, `pairs`, `ipairs`, `next`, `select`, `unpack`
- `pcall`, `xpcall`, `error`, `assert`

**Removed:**

- `os.execute`, `os.remove`, `os.rename`, `os.exit`, `os.getenv`, `os.tmpname`
- `io` — entire library
- `loadfile`, `dofile` — no loading external files
- `require` — no module loading
- `debug` — entire library
- `package` — entire library

### Rate Limiting

1. **Per-peer rate limit**: Max 30 invocations per minute per remote peer (configurable)
2. **Global rate limit**: Max 120 total script executions per minute (configurable)
3. **Per-function `@rate_limit`**: Annotations in data functions for custom per-function limits

### Resource Limits

| Resource | Limit | Rationale |
| -- | -- | -- |
| Execution time | 5s (default) | Prevents infinite loops |
| Memory | 10MB per VM | Prevents memory exhaustion |
| Registry max size | Derived from MaxMemoryMB | Caps table/string allocations |
| HTTP requests | 3 per invocation | Prevents amplification attacks |
| HTTP response size | 1MB per request | Prevents memory exhaustion |
| KV store | 64KB per script | Prevents disk exhaustion |
| Reply message size | 4KB | Chat messages should be short |

### No Filesystem Access

Scripts cannot read or write files. The `io` library is removed. The only persistent storage is `goop.kv`.

### No Network Access Beyond HTTP

Scripts can make outbound HTTP/HTTPS requests via `goop.http` but cannot open raw sockets, make DNS queries, or establish P2P connections. The HTTP client blocks requests to private/loopback addresses.

---

## Configuration

```json
{
    "lua": {
        "enabled": false,
        "script_dir": "site/lua",
        "timeout_seconds": 5,
        "max_memory_mb": 10,
        "rate_limit_per_peer": 30,
        "rate_limit_global": 120,
        "http_enabled": true,
        "kv_enabled": true
    }
}
```

| Field | Type | Default | Description |
| -- | -- | -- | -- |
| `enabled` | bool | `false` | Master switch for Lua scripting |
| `script_dir` | string | `"site/lua"` | Path to script directory (relative to peer folder) |
| `timeout_seconds` | int | `5` | Max execution time per invocation |
| `max_memory_mb` | int | `10` | Max memory per Lua VM (must be 1..1024) |
| `rate_limit_per_peer` | int | `30` | Max invocations per remote peer per minute |
| `rate_limit_global` | int | `120` | Max total invocations per minute |
| `http_enabled` | bool | `true` | Allow scripts to make HTTP requests |
| `kv_enabled` | bool | `true` | Allow persistent key-value storage |

Disabled by default. The peer must explicitly opt in.

---

## Implementation

### Package Structure

```bash
internal/lua/
    engine.go      -- Engine: loads scripts, manages proto map, handles reload
    sandbox.go     -- VM creation with restricted stdlib, goop.* injection
    api.go         -- goop.http, goop.json, goop.kv, goop.log implementations
    memlimit.go    -- Process-level memory monitor
    ratelimit.go   -- Sliding window rate limiter
```

### Hook Location

Chat commands are dispatched from `handleStream()` in `internal/chat/manager.go`. Data functions are dispatched from the data protocol handler when `op="lua-call"` or `op="lua-list"`.

### Templates as Full-Stack Starters

Templates can ship the backend too. A template becomes a full-stack application: frontend, database schema, and server-side logic.

Install a quiz template and you immediately have:

```bash
site/
  index.html          -- the quiz UI
  style.css           -- styling
  app.js              -- frontend logic (calls Lua functions)
  lua/
    functions/
      score-quiz.lua  -- server-side scoring
      leaderboard.lua -- server-side leaderboard
  schema.sql          -- tables for questions, answers, scores
```

The learning progression: **templates -> tweaking -> components -> custom scripts**. Each step requires a little more understanding and gives a little more control.

---

## Template Example: Tic-Tac-Toe

### How It Works

The host peer installs the tic-tac-toe template. Their site becomes a game lobby. Visitors see open games and can start a challenge. All game state lives in the host's database. Moves are validated server-side by a Lua function — neither player can cheat.

```bash
Host peer (PeerA)                          Visitor (PeerB)
    |                                           |
    |  installs tic-tac-toe template            |
    |           PeerB visits /p/<peerA>/         |
    |  <--------------------------------------- |
    |  PeerB clicks "New Game"                  |
    |    lua-call: ttt({action:"new"})           |
    |  <--------------------------------------- |
    |    returns: {game_id, your_symbol:"O"}     |
    |  ---------------------------------------> |
    |                                           |
    |  PeerA clicks cell (makes move)           |
    |    lua-call: move({game_id, position})     |
    |    Lua validates: correct turn, cell empty |
    |    Updates board, checks win/draw         |
    |                                           |
    |           PeerB polls for update           |
    |    lua-call: ttt({action:"state"})         |
    |  <--------------------------------------- |
    |    returns: {board, turn, status}          |
    |  ---------------------------------------> |
```

### Why Server-Side Validation Matters

Without it, a malicious visitor could play out of turn, overwrite occupied cells, claim fake wins, or modify opponent's moves. The Lua `move` function is the referee.

### Database Schema

```sql
CREATE TABLE games (
    _id          INTEGER PRIMARY KEY AUTOINCREMENT,
    _owner       TEXT NOT NULL,              -- host peer ID (always X)
    _owner_email TEXT DEFAULT '',
    _created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    _updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    challenger   TEXT NOT NULL DEFAULT '',    -- visitor peer ID (always O)
    challenger_label TEXT DEFAULT '',
    board        TEXT NOT NULL DEFAULT '---------',  -- 9 chars: X, O, or -
    turn         TEXT NOT NULL DEFAULT 'X',
    status       TEXT NOT NULL DEFAULT 'waiting',
                 -- waiting | playing | won_x | won_o | draw
    winner       TEXT DEFAULT ''
);
```

**Board encoding:** 9-character string, each char is `X`, `O`, or `-`. Positions 0-8 map left-to-right, top-to-bottom. Atomic — one column, one UPDATE, no race conditions.

### Lua Functions

**`move.lua`** — Core game logic. Validates moves, updates board, checks for win/draw:

```lua
--- Make a move in a tic-tac-toe game
function call(request)
    local p = request.params
    local game_id = p.game_id
    local pos = tonumber(p.position)  -- 0-8

    if not game_id or not pos then error("game_id and position required") end
    if pos < 0 or pos > 8 then error("position must be 0-8") end

    -- Load the game
    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, board, turn, status FROM games WHERE _id = ?",
        game_id)
    if not rows or #rows == 0 then error("game not found") end
    local game = rows[1]

    if game.status ~= "playing" then
        return { error = "game is not in progress", status = game.status }
    end

    -- Determine caller's symbol
    local symbol
    if goop.peer.id == game._owner then symbol = "X"
    elseif goop.peer.id == game.challenger then symbol = "O"
    else return { error = "you are not a player in this game" } end

    if game.turn ~= symbol then return { error = "not your turn" } end

    -- Check cell is empty and place move
    local idx = pos + 1  -- Lua is 1-indexed
    if string.sub(game.board, idx, idx) ~= "-" then
        return { error = "cell is already occupied" }
    end

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
        winner_id = (winner == "X") and game._owner or game.challenger
    elseif not string.find(new_board, "-") then
        new_status = "draw"
        new_turn = ""
    end

    goop.db.exec(
        "UPDATE games SET board = ?, turn = ?, status = ?, winner = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        new_board, new_turn, new_status, winner_id, game_id)

    return {
        board = new_board, turn = new_turn,
        status = new_status, winner = winner_id, your_symbol = symbol
    }
end

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
        if a ~= "-" and a == b2 and b2 == c then return a end
    end
    return nil
end
```

**`ttt.lua`** — Game management (new game, state queries, lobby):

```lua
--- Tic-tac-toe game management
function call(request)
    local action = request.params.action
    if action == "new" then return new_game()
    elseif action == "state" then return game_state(request.params.game_id)
    elseif action == "lobby" then return lobby()
    elseif action == "cancel" then return cancel_game(request.params.game_id)
    else error("unknown action: " .. tostring(action)) end
end

function new_game()
    local existing = goop.db.query(
        "SELECT _id FROM games WHERE challenger = ? AND status IN ('waiting', 'playing')",
        goop.peer.id)
    if existing and #existing > 0 then
        return { error = "you already have an active game", game_id = existing[1]._id }
    end

    goop.db.exec(
        "INSERT INTO games (_owner, challenger, challenger_label, status) VALUES (?, ?, ?, 'waiting')",
        goop.self.id, goop.peer.id, goop.peer.label)

    local id = goop.db.scalar("SELECT MAX(_id) FROM games WHERE challenger = ?", goop.peer.id)
    return { game_id = id, your_symbol = "O", status = "waiting" }
end

function game_state(game_id)
    if not game_id then error("game_id required") end
    local rows = goop.db.query(
        "SELECT * FROM games WHERE _id = ?", game_id)
    if not rows or #rows == 0 then error("game not found") end
    local g = rows[1]

    local your_symbol = ""
    if goop.peer.id == g._owner then your_symbol = "X"
    elseif goop.peer.id == g.challenger then your_symbol = "O" end

    return {
        game_id = g._id, board = g.board, turn = g.turn,
        status = g.status, winner = g.winner, your_symbol = your_symbol,
        challenger_label = g.challenger_label, created_at = g._created_at
    }
end

function lobby()
    local games = goop.db.query(
        "SELECT _id, challenger, challenger_label, board, turn, status, winner, _created_at FROM games ORDER BY _id DESC LIMIT 20")
    return { games = games or {} }
end
```

### Frontend

The frontend polls for updates (2-second interval while waiting for opponent's turn). When the group protocol is implemented, polling can be replaced with push notifications:

```javascript
// Before (polling):
setInterval(function () { checkState(gameId); }, 2000);

// After (group protocol):
Goop.group.join("ttt-" + gameId, function (msg) {
    if (msg.type === "move") renderBoard(msg.payload);
});
```

The database schema, Lua functions, and game logic don't change — only the transport layer upgrades.

### File Structure

```bash
internal/sitetemplates/tictactoe/
    manifest.json
    schema.sql
    index.html
    css/style.css
    js/app.js
    lua/functions/
        move.lua
        ttt.lua
```

### Edge Cases

| Case | Handling |
| -- | -- |
| Host never accepts challenge | Game stays `waiting`. Visitor can cancel. |
| Player disconnects mid-game | Game persists in DB. Resumes when they revisit. |
| Host goes offline | Visitor can't reach site (ephemeral web). Resumes when host returns. |
| Player moves twice | `move.lua` checks `game.turn != symbol`, returns error. |
| Two tabs open | Polls independently. Moves are atomic. No corruption. |
| Non-player calls move | Checks `goop.peer.id` against `_owner` and `challenger`. |

### Why This Template Matters

Tic-tac-toe is trivial as a game. As a template, it's a blueprint for server-side validation, turn-based multiplayer, polling (upgradeable to push), lobby/matchmaking, game history, and full-stack template architecture. Every pattern applies to chess, checkers, Connect Four, card games, etc.
