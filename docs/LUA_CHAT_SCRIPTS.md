# Lua Scripting

## Overview

Lua scripting turns every Goop2 peer into a programmable node. It has two faces:

**Phase 1 — Chat commands.** When a remote peer sends a direct message starting with `!`, the local chat manager dispatches it to a Lua script and replies with the result. Think `!weather London` → `"London: 12°C, partly cloudy"`. This works today over the existing `/goop/chat/1.0.0` protocol. No new infrastructure.

**Phase 2 — Data functions.** Lua scripts become server-side compute for the peer's ephemeral site. A visitor's browser calls a Lua function on the host peer via `/goop/data/1.0.0` and gets structured JSON back. This gives peers what they're currently missing — backend logic. A quiz site can score answers server-side so visitors can't cheat. A marketplace peer can validate bids. A game host can enforce rules.

Both phases share the same Lua engine, sandbox, and script directory. Phase 1 is the starting point. Phase 2 is the payoff.

### Why Lua

- Tiny, embeddable, battle-tested (30 years of game/plugin scripting)
- Go has mature embedding libraries (`gopher-lua`)
- Sandboxing is straightforward — disable `os`, `io`, `loadfile`, `dofile` at the VM level
- Scripts are readable by non-programmers
- Fast startup — a Lua VM initializes in microseconds

### Why Chat

Direct messages already flow between peers with verified cryptographic identity. The sender's peer ID is authenticated by the libp2p handshake. There's no need for API keys, tokens, or auth headers. The chat protocol is the simplest possible transport for request/response interactions between peers.

---

## Script Location & Lifecycle

### Where Scripts Live

```
{peerDir}/
  site/
    lua/
      weather.lua
      time.lua
      help.lua
      ...
    index.html
    style.css
```

Scripts live in `site/lua/` inside the peer's directory. Each `.lua` file in this directory is one command. The filename (minus extension) is the command name:

| File | Command |
|------|---------|
| `site/lua/weather.lua` | `!weather` |
| `site/lua/time.lua` | `!time` |
| `site/lua/help.lua` | `!help` |
| `site/lua/db-query.lua` | `!db-query` |

### Exclusion from Site Serving

The `lua/` directory must be **excluded from site serving**. These are executable scripts, not static content — they should never be served to visitors via `/goop/site/1.0.0`.

In `handleSiteStream()` (`internal/p2p/site.go`), add a path check before serving:

```go
// Reject requests for lua/ directory
if strings.HasPrefix(clean, "lua/") || clean == "lua" {
    fmt.Fprintf(s, "ERR forbidden\n")
    return
}
```

This keeps scripts private to the peer that owns them while the rest of the site folder remains publicly servable.

### Loading & Reloading

**Initial load**: When the chat manager starts (or when Lua scripting is first enabled), scan `site/lua/*.lua` and compile each file into a `*lua.FunctionProto`. Store compiled protos in a `map[string]*lua.FunctionProto` keyed by command name.

**Hot reload**: Watch `site/lua/` with `fsnotify`. On file change:
1. Re-compile the changed `.lua` file
2. Replace the entry in the proto map
3. Log the reload

On file deletion, remove the command. On new file creation, add it. No restart required.

**Compilation errors**: Log the error, skip the script. The previous working version (if any) remains active. Never crash on a bad script.

---

## Execution Model

### Dispatch Flow

```
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
    |  "London: 12°C, partly cloudy"     |
    | <--------------------------------- |
    |                                    |
```

### Message Parsing

When `handleStream()` receives a `MessageTypeDirect` message:

1. Trim whitespace from `msg.Content`
2. If it doesn't start with `!`, process as normal chat message
3. Split on first space: `command` and `args`
4. Strip the `!` prefix from command
5. Look up command in the script map
6. If not found, reply with `Unknown command: !<command>. Try !help`
7. If found, execute the script

### One VM Per Invocation

Each script execution gets a **fresh Lua VM**. No state leaks between invocations. No state leaks between different callers. This is the simplest model and the most secure.

The cost is real but small — `gopher-lua` creates a new VM in ~50μs. For chat-frequency interactions (seconds between messages), this is negligible.

### Timeouts

Every script execution has a hard timeout (default: 5 seconds, configurable). Implemented via a context-aware goroutine:

```go
ctx, cancel := context.WithTimeout(ctx, cfg.Lua.Timeout)
defer cancel()

done := make(chan string, 1)
go func() {
    result := runScript(L, proto, args)
    done <- result
}()

select {
case result := <-done:
    // send reply
case <-ctx.Done():
    L.Close() // kills the VM
    // send timeout error reply
}
```

If a script exceeds the timeout, the VM is forcibly closed and the sender receives an error message.

### Concurrency

Multiple peers can invoke scripts simultaneously. Each gets its own VM, so there's no locking beyond the script map's read lock. The script map uses `sync.RWMutex` — reads (execution) are concurrent, writes (reload) are exclusive.

---

## Lua API Reference

Scripts interact with Goop2 through a `goop` table injected into each VM before execution. The API is deliberately minimal — scripts can read context, make HTTP requests, and return text. They cannot access the filesystem, spawn processes, or modify peer state directly.

### Script Entry Point

A script is a Lua file that defines a `handle` function:

```lua
function handle(args)
    return "You said: " .. args
end
```

The `handle` function receives the argument string (everything after the command name) and returns a string that becomes the reply message. If `handle` returns `nil` or an empty string, no reply is sent.

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

HTTP client for external API calls (weather services, etc.). Subject to timeout and sandboxing.

```lua
-- GET request
local status, body, err = goop.http.get(url)
local status, body, err = goop.http.get(url, {["Authorization"] = "Bearer ..."})

-- POST request
local status, body, err = goop.http.post(url, content_type, body)
local status, body, err = goop.http.post(url, content_type, body, {["X-Custom"] = "value"})
```

- `status`: HTTP status code (number), or 0 on error
- `body`: response body (string), or "" on error
- `err`: error message (string), or nil on success
- Requests inherit the script's timeout — a 5-second script timeout means HTTP requests must also complete within that window
- Only `http://` and `https://` schemes are allowed
- Requests to `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, and `::1` are blocked (no SSRF against local services)

### `goop.log`

Logging visible in the Goop2 application log.

```lua
goop.log.info("processing request from " .. goop.peer.id)
goop.log.warn("API key missing, using fallback")
goop.log.error("failed to parse response")
```

### `goop.json`

JSON encoding/decoding for API responses.

```lua
local tbl = goop.json.decode('{"temp": 12, "city": "London"}')
print(tbl.temp)  -- 12

local str = goop.json.encode({temp = 12, city = "London"})
-- '{"city":"London","temp":12}'
```

### `goop.kv`

Simple persistent key-value store scoped per script. Backed by a file in `site/lua/.state/<scriptname>.json`. Survives restarts. Useful for API keys, counters, cached data.

```lua
goop.kv.set("api_key", "abc123")
local key = goop.kv.get("api_key")  -- "abc123"
goop.kv.del("api_key")
```

- Keys and values are strings
- Maximum 1000 keys per script, 64KB total
- The `.state/` directory is also excluded from site serving

---

## Example Scripts

### help.lua

Lists all available commands on this peer.

```lua
function handle(args)
    local commands = goop.commands()  -- returns list of command names
    local lines = {"Available commands:"}
    for _, cmd in ipairs(commands) do
        table.insert(lines, "  !" .. cmd)
    end
    return table.concat(lines, "\n")
end
```

`goop.commands()` returns a Lua table (array) of all loaded command names.

### time.lua

Returns the current time in a given timezone.

```lua
function handle(args)
    local tz = args ~= "" and args or "UTC"
    local now = os.date("!%Y-%m-%d %H:%M:%S UTC")
    return "Current time: " .. now
end
```

Note: `os.date` and `os.time` are the only `os.*` functions available in the sandbox. `os.execute`, `os.remove`, `os.rename`, etc. are removed.

### weather.lua

Fetches weather from an external API.

```lua
function handle(args)
    if args == "" then
        return "Usage: !weather <city>"
    end

    local key = goop.kv.get("api_key")
    if not key then
        return "Weather API key not configured. Owner must set it."
    end

    local url = "https://api.openweathermap.org/data/2.5/weather"
        .. "?q=" .. args
        .. "&appid=" .. key
        .. "&units=metric"

    local status, body, err = goop.http.get(url)
    if err then
        return "Error fetching weather: " .. err
    end
    if status ~= 200 then
        return "Weather API returned status " .. tostring(status)
    end

    local data = goop.json.decode(body)
    return string.format("%s: %s°C, %s",
        data.name,
        tostring(math.floor(data.main.temp)),
        data.weather[1].description
    )
end
```

### greet.lua

Responds differently based on who's calling.

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

Each Lua VM is created with a restricted set of standard libraries. The following are **available**:

- `string` — full string library
- `table` — full table library
- `math` — full math library
- `os.time`, `os.date`, `os.clock` — time functions only
- `tonumber`, `tostring`, `type`, `pairs`, `ipairs`, `next`, `select`, `unpack`
- `pcall`, `xpcall`, `error`, `assert`
- `string.format`, `string.find`, `string.match`, `string.gmatch`, `string.gsub`

The following are **removed**:

- `os.execute`, `os.remove`, `os.rename`, `os.exit`, `os.getenv`, `os.tmpname`
- `io` — entire library
- `loadfile`, `dofile` — no loading external files
- `require` — no module loading
- `debug` — entire library
- `package` — entire library
- `rawset`, `rawget` on restricted tables

### Rate Limiting

Two levels of rate limiting protect against abuse:

1. **Per-peer rate limit**: A remote peer can invoke at most **10 commands per minute** on any given local peer. Excess invocations receive a "Rate limited, try again later" reply.

2. **Global rate limit**: Total script executions across all remote peers are capped at **60 per minute**. This protects against distributed abuse.

Rate limits are tracked with a sliding window counter. Configurable in the config file.

### Resource Limits

| Resource | Limit | Rationale |
|----------|-------|-----------|
| Execution time | 5s (default) | Prevents infinite loops |
| Memory | 10MB per VM | Prevents memory exhaustion |
| HTTP requests | 3 per invocation | Prevents amplification attacks |
| HTTP response size | 1MB per request | Prevents memory exhaustion |
| KV store | 64KB per script | Prevents disk exhaustion |
| Reply message size | 4KB | Chat messages should be short |

### No Filesystem Access

Scripts cannot read or write files. The `io` library is removed entirely. The only persistent storage is the `goop.kv` API, which is a controlled, bounded key-value store.

### No Network Access Beyond HTTP

Scripts can make outbound HTTP/HTTPS requests via `goop.http` but cannot open raw sockets, make DNS queries, or establish P2P connections. The HTTP client blocks requests to private/loopback addresses.

---

## Configuration

New `Lua` section in the config file:

```json
{
    "lua": {
        "enabled": false,
        "script_dir": "site/lua",
        "timeout_seconds": 5,
        "max_memory_mb": 10,
        "rate_limit_per_peer": 10,
        "rate_limit_global": 60,
        "http_enabled": true,
        "kv_enabled": true
    }
}
```

### Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Master switch for Lua scripting |
| `script_dir` | string | `"site/lua"` | Path to script directory, relative to peer folder |
| `timeout_seconds` | int | `5` | Max execution time per script invocation |
| `max_memory_mb` | int | `10` | Max memory per Lua VM |
| `rate_limit_per_peer` | int | `10` | Max invocations per remote peer per minute |
| `rate_limit_global` | int | `60` | Max total invocations per minute |
| `http_enabled` | bool | `true` | Allow scripts to make HTTP requests |
| `kv_enabled` | bool | `true` | Allow scripts to use persistent key-value storage |

Disabled by default. The peer must explicitly opt in to running scripts.

### Config Struct

Add to `internal/config/config.go`:

```go
type Lua struct {
    Enabled          bool   `json:"enabled"`
    ScriptDir        string `json:"script_dir"`
    TimeoutSeconds   int    `json:"timeout_seconds"`
    MaxMemoryMB      int    `json:"max_memory_mb"`
    RateLimitPerPeer int    `json:"rate_limit_per_peer"`
    RateLimitGlobal  int    `json:"rate_limit_global"`
    HTTPEnabled      bool   `json:"http_enabled"`
    KVEnabled        bool   `json:"kv_enabled"`
}
```

Add `Lua Lua` field to the top-level `Config` struct with `json:"lua"` tag.

---

## Implementation Notes

### Go Library

Use **[gopher-lua](https://github.com/yuin/gopher-lua)** — the most mature pure-Go Lua 5.1 VM. No CGo, no external dependencies, compiles cleanly on all platforms.

Key APIs:
- `lua.NewState()` — creates a VM
- `L.SetGlobal()` — injects the `goop.*` table
- `L.DoString()` / `L.DoCompiledFunction()` — runs compiled scripts
- `L.Close()` — destroys the VM (can be called from another goroutine to enforce timeout)

### New Package: `internal/lua`

```
internal/lua/
    engine.go      -- Engine type: loads scripts, manages proto map, handles reload
    sandbox.go     -- VM creation with restricted stdlib, goop.* injection
    api.go         -- goop.http, goop.json, goop.kv, goop.log implementations
    ratelimit.go   -- sliding window rate limiter
```

### Hook Location: Chat Manager

The integration point is `handleStream()` in `internal/chat/manager.go`. After the message is decoded and validated:

```go
func (m *Manager) handleStream(stream network.Stream) {
    // ... existing decode + validate logic ...

    // Lua dispatch (after addMessage, before return)
    if msg.Type == MessageTypeDirect && strings.HasPrefix(msg.Content, "!") {
        if m.luaEngine != nil {
            go m.luaEngine.Dispatch(context.Background(), &msg, m)
        }
    }
}
```

The `Dispatch` method parses the command, checks rate limits, runs the script, and calls `m.SendDirect()` with the result. It runs in a goroutine so it doesn't block the stream handler.

### Engine Interface

```go
type Engine struct {
    mu       sync.RWMutex
    scripts  map[string]*lua.FunctionProto
    cfg      config.Lua
    kv       *kvStore
    watcher  *fsnotify.Watcher
    limiter  *rateLimiter
}

func NewEngine(cfg config.Lua, peerDir string) (*Engine, error)
func (e *Engine) Dispatch(ctx context.Context, msg *chat.Message, sender DirectSender)
func (e *Engine) Commands() []string
func (e *Engine) Close() error
```

`DirectSender` is an interface so the engine doesn't depend on the full chat manager:

```go
type DirectSender interface {
    SendDirect(ctx context.Context, toPeerID, content string) error
}
```

### Site Exclusion

In `handleSiteStream()` (`internal/p2p/site.go`), add the exclusion check after path normalization and before the file read:

```go
clean := strings.TrimPrefix(filepath.Clean(reqPath), "/")
// ...existing path traversal check...

// Exclude lua scripts directory from site serving
if strings.HasPrefix(clean, "lua/") || clean == "lua" {
    fmt.Fprintf(s, "ERR forbidden\n")
    return
}
```

Also exclude `.state/` within lua:

```go
if strings.HasPrefix(clean, "lua/.state/") || clean == "lua/.state" {
    fmt.Fprintf(s, "ERR forbidden\n")
    return
}
```

### Wiring

In the application startup (wherever the chat manager and P2P node are initialized):

1. Load config
2. If `cfg.Lua.Enabled`, create `lua.NewEngine(cfg.Lua, peerDir)`
3. Set `chatManager.luaEngine = engine`
4. On shutdown, call `engine.Close()`

---

## Phase 2: Lua Data Functions

### The Problem

Right now a peer's site is static HTML/JS/CSS served over `/goop/site/1.0.0`. The data protocol (`/goop/data/1.0.0`) gives structured access to the peer's SQLite database. Between these two, peers can build interactive sites — but all logic runs client-side, in the visitor's browser.

This is fine until you need the host to enforce rules. A quiz where the visitor's browser knows the answers isn't much of a quiz. A marketplace where the buyer's browser validates its own bid isn't trustworthy. A game where the client decides who won isn't fair.

What's missing is **server-side compute** — logic that runs on the host peer's machine, where the visitor can't inspect or tamper with it.

### The Idea

Lua scripts in `site/lua/` gain a second entry point. In addition to `handle(args)` for chat commands, a script can export `call(request)` for data protocol invocations:

```lua
-- site/lua/score-quiz.lua

-- Chat interface (Phase 1)
function handle(args)
    return "This is a data function. Visit my site to take the quiz."
end

-- Data interface (Phase 2)
function call(request)
    local answers = request.params.answers    -- table from visitor
    local quiz_id = request.params.quiz_id

    -- Load correct answers from the host's database
    local correct = goop.db.query(
        "SELECT question_id, answer FROM quiz_answers WHERE quiz_id = ?",
        quiz_id
    )

    local score = 0
    for _, row in ipairs(correct) do
        if answers[row.question_id] == row.answer then
            score = score + 1
        end
    end

    return {
        score = score,
        total = #correct,
        passed = score >= math.floor(#correct * 0.7)
    }
end
```

The visitor's site JavaScript calls this function:

```javascript
// In the host peer's site template, running in the visitor's browser
const result = await goopData.call("score-quiz", {
    quiz_id: "midterm-2026",
    answers: { q1: "B", q2: "A", q3: "D" }
});
// result = { score: 2, total: 3, passed: false }
```

### How It Works

The data protocol already supports structured request/response between peers. Data functions add a new operation type alongside the existing database operations:

```
Visitor browser
    → goopData.call("score-quiz", params)
    → local Goop2 viewer
    → /goop/data/1.0.0 stream to host peer
    → host peer's data handler sees op="lua-call"
    → dispatches to Lua engine
    → Lua script runs with goop.db access
    → returns structured result
    → JSON response back through the stream
    → visitor's browser receives result
```

### Request/Response Format

**Request** (JSON over data protocol stream):

```json
{
    "op": "lua-call",
    "function": "score-quiz",
    "params": {
        "quiz_id": "midterm-2026",
        "answers": { "q1": "B", "q2": "A", "q3": "D" }
    }
}
```

**Response**:

```json
{
    "ok": true,
    "result": {
        "score": 2,
        "total": 3,
        "passed": false
    }
}
```

**Error response**:

```json
{
    "ok": false,
    "error": "function not found: score-quiz"
}
```

### Extended API: `goop.db`

Data functions get an additional API that chat commands don't — read access to the host peer's SQLite database:

```lua
-- Read-only query, returns array of row tables
local rows = goop.db.query("SELECT * FROM quiz_answers WHERE quiz_id = ?", quiz_id)

-- Single value
local count = goop.db.scalar("SELECT COUNT(*) FROM submissions WHERE peer_id = ?", goop.peer.id)
```

- **Read-only**. Scripts cannot INSERT, UPDATE, DELETE, or run DDL. The database connection uses SQLite's read-only mode.
- **Scoped to the peer's own database**. No cross-peer queries.
- **Parameterized queries only**. The `query` and `scalar` functions use prepared statements — no string concatenation, no SQL injection.
- **Row limit**: queries return at most 1000 rows. Result sets are capped at 1MB serialized.
- **The caller's peer ID is always available** via `goop.peer.id`, so scripts can filter by visitor identity without trusting client-supplied values.

If a script needs to write data (record a quiz submission, log a bid), it returns the data to the caller and the caller writes it through the normal data protocol — where the peer ID is stamped by the system, not the script. This keeps the write path honest.

### Capability Discovery

Unlike chat commands where `!help` is enough, data functions need programmatic discovery. A visiting peer's site JavaScript needs to know what functions are available before calling them.

A well-known data operation handles this:

```json
{ "op": "lua-list" }
```

Returns:

```json
{
    "ok": true,
    "functions": [
        {
            "name": "score-quiz",
            "description": "Score a quiz submission"
        },
        {
            "name": "get-products",
            "description": "List available products"
        }
    ]
}
```

The `description` comes from a comment at the top of each script:

```lua
--- Score a quiz submission
function call(request)
    ...
end
```

### Examples

**Product catalog with server-side filtering:**

```lua
--- Search products by category and price range
function call(request)
    local p = request.params
    local rows = goop.db.query(
        "SELECT id, name, price, description FROM products WHERE category = ? AND price BETWEEN ? AND ? ORDER BY price LIMIT 50",
        p.category, p.min_price or 0, p.max_price or 999999
    )
    return { products = rows }
end
```

**Leaderboard with anti-cheat:**

```lua
--- Get leaderboard for a game
function call(request)
    local game_id = request.params.game_id
    local scores = goop.db.query(
        "SELECT peer_id, score, submitted_at FROM scores WHERE game_id = ? ORDER BY score DESC LIMIT 20",
        game_id
    )
    return {
        game_id = game_id,
        leaderboard = scores
    }
end
```

**Rate-limited API proxy:**

```lua
--- Get weather data (caches results, avoids exposing API key)
function call(request)
    local city = request.params.city
    if not city or city == "" then
        error("city parameter required")
    end

    -- Check cache
    local cached = goop.kv.get("weather:" .. city)
    if cached then
        local data = goop.json.decode(cached)
        if os.time() - data.fetched < 300 then  -- 5 minute cache
            return data
        end
    end

    -- Fetch fresh data (API key stays on host, never sent to visitor)
    local key = goop.kv.get("owm_api_key")
    local status, body = goop.http.get(
        "https://api.openweathermap.org/data/2.5/weather?q=" .. city .. "&appid=" .. key .. "&units=metric"
    )

    local weather = goop.json.decode(body)
    local result = {
        city = weather.name,
        temp = weather.main.temp,
        description = weather.weather[1].description,
        fetched = os.time()
    }

    goop.kv.set("weather:" .. city, goop.json.encode(result))
    return result
end
```

This last example shows something important: the API key lives on the host peer and is never exposed to the visitor. The Lua function acts as a proxy — the visitor gets weather data, the host keeps control of the key. This is a pattern that's impossible with client-side-only sites.

### Security Additions for Phase 2

Data functions inherit all Phase 1 security (sandbox, timeouts, rate limits, resource caps) with these additions:

| Concern | Mitigation |
|---------|------------|
| SQL injection | Parameterized queries only; `goop.db.query` uses prepared statements |
| Data exfiltration | Read-only database access; no writes from scripts |
| Large result sets | 1000 row limit, 1MB serialized response cap |
| Function enumeration | `lua-list` only returns functions that define `call()`; chat-only scripts are hidden |
| Abuse from visitors | Same rate limiting as chat: per-peer and global caps apply |

### What This Makes Possible

With Phase 1 (chat commands) and Phase 2 (data functions) together, a Goop2 peer becomes a full application server:

- **Static assets** via `/goop/site/1.0.0` — the frontend
- **Database** via `/goop/data/1.0.0` — the data layer
- **Server-side logic** via Lua data functions — the backend
- **Peer identity** via libp2p — authentication for free

A peer can host a quiz app, a store, a game, a dashboard — anything that needs a frontend, a database, and server-side logic. The visitor's browser loads the site, calls Lua functions for server-side work, and reads/writes data through the data protocol. All running on the host's machine, all authenticated by the mesh, no cloud infrastructure required.

This is the completion of the peer-as-platform model. The ephemeral web gets a backend.

---

## Templates as Full-Stack Starters

### The Idea

Templates already drop files into a peer's site folder — HTML, CSS, JavaScript. With Lua scripting, templates can ship the backend too. A template becomes a full-stack application in a zip file: frontend, database schema, and server-side logic, all wired together and working out of the box.

Install a quiz template, and you immediately have:

```
site/
  index.html          ← the quiz UI
  style.css           ← styling
  app.js              ← frontend logic (calls Lua functions)
  lua/
    score-quiz.lua    ← server-side scoring
    leaderboard.lua   ← server-side leaderboard
  schema.sql          ← tables for questions, answers, scores
```

The peer doesn't need to understand Lua, SQL, or JavaScript to have a working quiz site. They install the template, add their questions through the UI, and visitors can take the quiz with server-side scoring — immediately.

### Learning by Tweaking

This is the real value. Instead of reading docs and writing a Lua script from scratch, a peer starts with working code and modifies it:

1. **Install a template** — everything works, zero configuration
2. **Read the Lua script** — it's 30 lines, readable, well-commented
3. **Change something small** — adjust the passing score from 70% to 50%
4. **See the result immediately** — hot reload picks up the change
5. **Try something bigger** — add a time bonus to the scoring logic
6. **Build your own** — now you understand the pattern, write a new script from scratch

This is how people actually learn. Not from documentation — from working examples they can break and fix. Templates provide the working example. Lua provides the readability. Hot reload provides the feedback loop.

### Template Complexity Tiers

Not every template needs Lua. Templates naturally fall into tiers of complexity:

| Tier | Ships with | Example |
|------|-----------|---------|
| **Static** | HTML, CSS, JS | Personal homepage, portfolio |
| **Data** | Static + schema.sql | Guestbook, link collection |
| **Full-stack** | Data + lua/ scripts | Quiz, marketplace, game |

A peer's journey might follow the tiers: start with a static template, graduate to one with a database, then try a full-stack template with Lua. Each tier builds on the last. By the time they reach full-stack, they've already seen how the site folder, the database, and the data protocol work — Lua is just the next piece.

### Specialization Over Templates

Once a peer understands the pieces, they don't need complete templates anymore. They need components:

- "I want to add server-side scoring to my existing quiz" → drop in `score-quiz.lua`
- "I want a leaderboard on my game site" → drop in `leaderboard.lua` + a schema migration
- "I want to proxy an API without exposing my key" → drop in `api-proxy.lua`

These are **Lua snippets** — single-purpose scripts that solve one problem. Smaller than templates, shareable between peers, composable. A peer might install a full-stack quiz template to start, then later replace the scoring script with a custom one, add a leaderboard script from someone else, and write their own analytics script.

The progression: **templates → tweaking → components → custom scripts**. Each step requires a little more understanding and gives a little more control. Nobody has to start from zero.
