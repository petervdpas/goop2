# Scripting with Lua

Goop2 includes an embedded Lua runtime that lets you add server-side logic to your peer. Lua scripts can power chat commands, validate data, compute scores, enforce game rules, and more.

## Enabling Lua

Add the `lua` section to your `goop.json`:

```json
{
  "lua": {
    "enabled": true,
    "script_dir": "site/lua",
    "timeout_seconds": 5,
    "max_memory_mb": 10
  }
}
```

## Script types

### Chat commands

Files in `site/lua/` (not in `functions/`) are chat commands. A visitor sends a direct message starting with `!` and the matching script runs.

File: `site/lua/hello.lua`

```lua
function handle(args)
    return "Hello, " .. (args ~= "" and args or "world") .. "!"
end
```

A visitor typing `!hello Alice` receives the response `Hello, Alice!`.

### Data functions

Files in `site/lua/functions/` are data functions. They are called from the browser via the `goop-data.js` library and return structured data.

File: `site/lua/functions/score-quiz.lua`

```lua
function call(request)
    local answers = request.params.answers
    local score = 0
    -- scoring logic here
    return { score = score, total = #answers }
end
```

Called from JavaScript:

```javascript
const result = await Goop.data.call("score-quiz", { answers: [...] });
```

## Available APIs

### goop.peer

Information about the calling peer:

```lua
goop.peer.id       -- Peer ID (e.g. "12D3Koo...")
goop.peer.label    -- Display name (if known)
```

### goop.self

Information about the local peer:

```lua
goop.self.id       -- Local peer ID
goop.self.label    -- Local display name
```

### goop.http

HTTP client (requires `http_enabled: true`):

```lua
local body, err = goop.http.get("https://api.example.com/data")
local body, err = goop.http.post("https://api.example.com/submit", {key = "val"})
```

Only `http://` and `https://` URLs are allowed. Requests to private/loopback addresses are blocked.

### goop.json

JSON encoding and decoding:

```lua
local obj = goop.json.decode('{"name":"Alice"}')
local str = goop.json.encode({name = "Bob"})
```

### goop.kv

Persistent key-value store (per script, requires `kv_enabled: true`):

```lua
goop.kv.set("api_key", "secret123")
local key = goop.kv.get("api_key")
goop.kv.del("api_key")
```

Limited to 1000 keys and 64 KB total per script.

### goop.log

Logging:

```lua
goop.log.info("processing request")
goop.log.warn("API key missing")
goop.log.error("connection failed")
```

### goop.db

Database access (data functions only):

```lua
local rows = goop.db.query("SELECT * FROM posts WHERE _owner = ?", goop.peer.id)
local count = goop.db.scalar("SELECT COUNT(*) FROM responses")
goop.db.exec("UPDATE games SET turn = ? WHERE _id = ?", "O", game_id)
```

### goop.commands()

Returns a list of all loaded chat commands.

## Security

Every Lua invocation runs in a fresh, sandboxed VM:

- **No filesystem access** -- `io`, `loadfile`, and `dofile` are disabled.
- **No module loading** -- `require` and `package` are disabled.
- **No shell execution** -- `os.execute`, `os.remove`, etc. are disabled.
- **Hard timeout** -- Default 5 seconds, configurable up to 60.
- **Memory limit** -- Default 10 MB per VM.
- **Rate limiting** -- Per-peer (30/min) and global (120/min) limits prevent abuse.

## Hot reload

Scripts are automatically reloaded when their files change. There is no need to restart the peer. If a script has a syntax error, the previous working version stays active and the error is logged.

## Example: weather command

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
        data.name,
        tostring(math.floor(data.main.temp)),
        data.weather[1].description)
end
```

## Example: game move validation

```lua
function call(request)
    local game_id = request.params.game_id
    local position = tonumber(request.params.position)

    local rows = goop.db.query("SELECT * FROM games WHERE _id = ?", game_id)
    if not rows or #rows == 0 then
        error("game not found")
    end

    local game = rows[1]
    if game.turn ~= goop.peer.id then
        return { error = "not your turn" }
    end

    local idx = position + 1
    if string.sub(game.board, idx, idx) ~= "-" then
        return { error = "cell occupied" }
    end

    local new_board = string.sub(game.board, 1, idx - 1)
                   .. "X"
                   .. string.sub(game.board, idx + 1)

    goop.db.exec("UPDATE games SET board = ?, turn = ? WHERE _id = ?",
        new_board, "O", game_id)

    return { board = new_board, turn = "O" }
end
```
