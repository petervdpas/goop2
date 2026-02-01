# Lua Chat Scripts

## Overview

Lua chat scripts turn every Goop2 peer into a programmable service endpoint. When a remote peer sends a direct message that starts with `!`, the local chat manager intercepts it, dispatches it to a matching Lua script, and returns the script's output as a reply message — all over the existing `/goop/chat/1.0.0` protocol.

Think of it as a per-peer API surfaced through chat commands. A peer running a weather script responds to `!weather London` with a forecast. A peer running a help script responds to `!help` with a list of available commands. No new protocols, no new infrastructure — just Lua scripts in a folder.

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
