# Lua Engine Internals

## Sandbox

<!-- STUB: Restricted standard libraries (no os, io, loadfile) -->
<!-- Injected globals: goop.* table with all APIs -->
<!-- Each function call gets its own invocation context (peerID, selfID) -->

## Invocation context

<!-- STUB: inv.peerID = the calling peer (remote viewer or self) -->
<!-- inv.selfID = the host peer -->
<!-- Role checks use peerID: if peerID == selfID → "owner", else look up in group -->

## goop.* API surface

<!-- STUB: Full list of what's in the sandbox -->
<!-- goop.orm(table) — ORM handle with find, insert, update, delete, etc. -->
<!-- goop.config(table, defaults) — config handle with get/set -->
<!-- goop.group.* — create, close, add, remove, set_role, members, grouptypes, list, send -->
<!-- goop.group.member.id, goop.group.member.role(), goop.group.is_member(), goop.group.owner() -->
<!-- goop.template.* — full manifest (name, description, category, icon, schemas, require_email, default_role) -->
<!-- goop.peer.id, goop.peer.label, goop.self.id, goop.self.label -->
<!-- goop.route(actions) — action dispatcher -->
<!-- goop.owner(fn) — wraps function to reject non-owners -->
<!-- goop.expr(table, expression) — safe SQL expression builder -->
<!-- goop.db.* — legacy raw SQL (query, scalar, exec) -->

## Rate limiting

<!-- STUB: Per-peer rate limiting -->
<!-- Default from config: rate_limit_per_peer -->
<!-- Override per script: --- @rate_limit N annotation -->
<!-- 0 = unlimited -->

## Engine lifecycle

<!-- STUB: Start, rescan (hot-reload on template apply), stop -->
<!-- EnsureLua callback: starts engine if not running, rescans functions dir -->
<!-- Scripts loaded from <peerDir>/site/lua/functions/*.lua -->

## Dispatch flow

<!-- STUB: goop.route() creates a dispatcher table -->
<!-- call(req) is the entry point — req has action + params -->
<!-- goop.owner(fn) wraps fn to check inv.peerID == inv.selfID -->

## ORM call flow

<!-- STUB: Lua goop.orm("table"):insert(data) -->
<!-- → Go schemaInsertFn → db.OrmInsert(table, peerID, email, data) -->
<!-- Direct SQLite call — no P2P hop for host-side Lua execution -->
<!-- Remote viewers trigger Lua via P2P data proxy → Lua runs on host → SQLite -->
