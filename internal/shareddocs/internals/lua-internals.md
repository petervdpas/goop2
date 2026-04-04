# Lua Engine Internals

## Sandbox

`internal/lua/engine.go`

Restricted standard libraries: no `os`, `io`, `loadfile`, or other filesystem/network access. The sandbox provides only safe string, table, math, and formatting functions.

Injected globals: the `goop.*` table with all APIs (ORM, groups, config, routing, etc.).

Each function call gets its own invocation context with `peerID` (the calling peer) and `selfID` (the host peer).

## Invocation context

- `inv.peerID` = the calling peer (remote viewer or self)
- `inv.selfID` = the host peer
- Role checks use peerID: if `peerID == selfID` → `"owner"`, else look up role in the template group

## goop.* API surface

### Data

- `goop.orm(table)` — ORM handle with: `find`, `find_one`, `get`, `get_by`, `list`, `pluck`, `exists`, `count`, `distinct`, `aggregate`, `insert`, `update`, `delete`, `update_where`, `delete_where`, `upsert`, `seed`, `validate` + `.columns` / `.access` properties
- `goop.config(table, defaults)` — config handle with `get` / `set`
- `goop.expr(table, expression)` — safe SQL expression builder
- `goop.db.*` — legacy raw SQL: `query`, `scalar`, `exec` (kept for backwards compat)

### Groups

- `goop.group.create(name, type, context, max)` — create a hosted group
- `goop.group.close(id)` — close a hosted group
- `goop.group.add(id, peer)` — add a peer to a group
- `goop.group.remove(id, peer)` — remove a peer
- `goop.group.set_role(id, peer, role)` — set member role
- `goop.group.members(id)` — list group members
- `goop.group.grouptypes()` — list registered group types
- `goop.group.list()` — list hosted groups
- `goop.group.send(id, payload)` — send message to group
- `goop.group.member.id` — calling peer's ID
- `goop.group.member.role()` — calling peer's role in the template group
- `goop.group.is_member()` — whether caller is a template group member
- `goop.group.owner()` — template group owner ID

### Identity

- `goop.peer.id` — calling peer's ID
- `goop.peer.label` — calling peer's display name
- `goop.self.id` — host peer's ID
- `goop.self.label` — host peer's display name

### Template

- `goop.template.name`, `goop.template.description`, `goop.template.category`, `goop.template.icon`
- `goop.template.schemas`, `goop.template.require_email`, `goop.template.default_role`

### Routing

- `goop.route(actions)` — action dispatcher table: maps action names to functions
- `goop.owner(fn)` — wraps function to reject non-owners (checks `inv.peerID == inv.selfID`)

### Chat

- `goop.chat.create(name, desc, max)` — create a chat room
- `goop.chat.close(id)` — close a chat room
- `goop.chat.rooms()` — list active rooms

## Rate limiting

Per-peer rate limiting enforced by the engine:

- Default from config: `rate_limit_per_peer` (requests per minute per peer per function)
- Override per script: `--- @rate_limit N` annotation in the leading comment block
- `0` = unlimited
- Global rate limit also configurable: `rate_limit_global`
- Checked before every function dispatch and `CallFunction` invocation

## Engine lifecycle

1. **Start**: `New(cfg)` creates engine, loads scripts from `<peerDir>/site/lua/functions/*.lua`
2. **Rescan**: `RescanFunctions()` hot-reloads scripts on template apply — called via `EnsureLua` callback
3. **Stop**: `Close()` shuts down the engine

Scripts are loaded from `<peerDir>/site/lua/functions/*.lua`. Each `.lua` file defines one or more functions accessible via `goop.route()`.

The `EnsureLua` callback (wired in `peer.go`) starts the engine if not running and rescans the functions directory.

## Dispatch flow

1. Remote viewer calls `/api/lua/<function>` with action + params
2. HTTP handler invokes engine's `Dispatch(callerID, function, request)`
3. Rate limit check (per-peer, per-function)
4. Engine creates invocation context with caller's peerID
5. `goop.route()` dispatcher table maps action to handler function
6. `goop.owner(fn)` wraps function to check `inv.peerID == inv.selfID`
7. Result returned as JSON

## ORM call flow

Lua `goop.orm("table"):insert(data)`:

1. Lua call → Go `schemaInsertFn`
2. → `db.OrmInsert(table, peerID, email, data)`
3. Direct SQLite call — no P2P hop for host-side Lua execution
4. Remote viewers trigger Lua via P2P data proxy → Lua runs on host → SQLite
