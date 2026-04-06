# Backlog History

Completed items moved from `backlog.md`.

---

## 2026-04-06

### internal/app/shared tests

Added `opts_test.go` with 5 cases for `NormalizeLocalViewer`:
- Port-only (`:8080` → `127.0.0.1:8080`), wildcard bind (`0.0.0.0:` → `127.0.0.1:`), explicit localhost, whitespace trimming, non-localhost IP passthrough

### internal/app/modes — no tests (reviewed)

All 4 files (bridge.go, peer.go, rendezvous.go, signaler.go) are pure orchestration wiring — no extractable pure functions. `timings.go` is constants only. Not worth testing.

### internal/app tests

Added `app_test.go` with 4 tests covering:
- `WaitTCP`: success (real listener), timeout (unreachable port)
- `setupMicroService`: skips on empty URL, calls configure on non-empty URL
- `run.go` and `timings.go` are pure orchestration/constants, not unit-testable

### internal/bridge tests

Added `client_test.go` with 6 tests covering:
- `New`: URL trimming, field assignment, DNS cache + HTTP client init
- `Register`: success (201 + session_id + headers verified), non-201 error
- `connectOnce`: receives presence events via WS, ignores unknown event types
- `Connect`: reconnects after WS failure with backoff (register → fail → register → succeed)

### internal/ui/viewmodels tests

Added `viewmodels_test.go` with 5 tests covering:
- `BuildPeerRow`: all fields mapped correctly (Content, Email, AvatarHash, VideoDisabled, ActiveTemplate, PublicKey, Verified, Reachable, LastSeen, Favorite), Offline derived from OfflineSince
- `BuildPeerRows`: empty map, sorted by ID, field mapping preserved
- Other files are pure struct definitions (no logic to test)

### internal/ui/render tests

Added `render_test.go` with 10 tests covering:
- `Highlight`: Go, JavaScript, HTML, CSS, unknown language (fallback), empty code
- `InitTemplates`: success, idempotent (sync.Once)
- `RenderStandalone`: renders named template with data, unknown template returns 500

### internal/viewer tests

Added `viewer_test.go` with 18 tests covering:
- `contentTypeForPath`: 10 cases (CSS, JS, HTML, HTM, SVG, case-insensitive, PNG sniff, JSON, text/binary fallback)
- `LogBuffer`: Write+Snapshot, partial line buffering, blank line skipping, ring overflow, default max, Subscribe, CancelSubscription (double cancel safe), ServeLogsJSON, ServeLogsJSON method not allowed, CR stripping
- `noCache` middleware: Cache-Control/Pragma/Expires headers
- `proxyPeerSite`: self short-circuit (content + headers + CSP), default index.html, not found, no content store (500), no/empty peer ID (404) — uses real libp2p host via `&p2p.Node{Host: h}`

### internal/content tests

Added `store_test.go` with 34 tests covering:
- NewStore (defaults, absolute path), Write+Read (round-trip, etag), Read NotFound
- Write: etag conflict, etag "none" for new files, image path enforcement, path traversal, dir conflict, auto-create parent dirs
- Delete/DeletePath: file, not found, recursive dir, non-recursive non-empty dir
- Mkdir: success, path traversal
- List: files+dirs, not found
- ListTree: nested items with depth, dirs-before-files sort
- Rename: success, not found, path traversal
- NormalizeDir: file→parent, directory→self, empty
- MkdirUnder: success, empty name, slash in name, dotdot
- Pure functions: normalizeRelPath (6 cases), etagBytes, cleanAbs path traversal

### internal/avatar tests

Added `avatar_test.go` with 21 tests covering:
- Store: NewStore (no avatar), Write+Read, ReadNoAvatar, Delete, DeleteNonExistent, HashChangesOnWrite, HashDeterministic, InitialHashFromExistingFile
- Pure functions: hashBytes length, extractInitials (8 cases incl. unicode), deterministicColor (determinism + diversity), InitialsSVG (content + empty label)
- Cache: PutAndGet, GetHashMismatch, GetEmptyHash, GetMissingPeer, GetAny, GetAnyMissing, Clear

### internal/config tests

Added `config_test.go` with 49 tests covering:
- `Default()` validates cleanly + key default values verified
- `Validate()` — every section: identity (empty key), paths (empty/equal), P2P (port range, empty mdns), presence (topic, TTL, heartbeat, heartbeat≥TTL, rendezvous-only), rendezvous host (port, bind), relay (requires host, port range, WS port, 5 negative timings), Lua (all 7 constraints + disabled skips validation)
- `validateWANRendezvous` — 11 cases (valid URLs, invalid scheme, no host, bind address, bad port)
- `stripBOM` — with/without BOM, short input
- `Load` — valid file, BOM, invalid JSON, validation failure, missing file
- `LoadPartial` — skips validation
- `Save` — valid round-trip, invalid rejected
- `Ensure` — creates default, loads existing

### Rendezvous WS reconnection state machine tests

Added `client_ws_test.go` with 15 tests covering:
- Pure functions: `isWSUnsupported` (6 cases), `isWSTooEarly` (3 cases), `wsBase` URL conversion (5 cases), `wsURL`, `wsProbeURL`
- `PublishWS` edge cases: no connection, buffer full, success
- `subscribeOnce` SSE parsing: valid messages, invalid/malformed messages skipped, non-2xx status
- `SubscribeEvents` reconnection: retries after server failures with backoff
- `ConnectWebSocket` state machine: receives WS messages, 425 retry (publish-first), reconnects after close, SSE fallback with WS probe switch-back
- `IsWebSocketConnected` initial state

### viewer route tests

Added 4 test files covering viewer route handlers with httptest:
- `helpers_test.go` — 17 tests for pure helper functions (normalizeRel, dirOf, atoiOrNeg, isImageExt, isValidTheme, formBool, safeCall, newToken, isLocalRequest, requireMethod, requireLocal, requireContentStore, writeJSON, topologyHandler, handleGet, handlePostAction, fetchServiceHealth)
- `home_test.go` — 8 tests for home routes (/ redirect, 404, /api/peers, /api/peers/favorite, /api/topology)
- `data_test.go` — 20 tests for data API routes (list tables, insert, find, find-one, count, exists, pluck, get-by, create/delete table, describe, set-policy, role, update-where, delete-where, upsert, orm-schema, export-schema)
- `site_api_test.go` — 8 tests for site API routes (list files, content read/write, delete, upload-local, no-store errors, method checks)
- `export_test.go` — 4 tests for extractZip (basic, wrapper stripping, path traversal rejection, invalid data)

Total: 57 new tests (routes package went from 23 to 80 passing tests).

### mq.Transport interface and field naming

- `internal/mq/transport.go` — interface extracted, file renamed from sender.go
- All manager fields: `mq mq.Transport` (consistent)
- All constructor params: `transport mq.Transport` (declarative)
- Viewer routes keep `mqMgr *mq.Manager` (correct — they need the concrete type)
- `mq.NopTransport{}` default in test managers

### ORM operations BDD feature

Added `tests/orm/orm.feature` + `tests/orm/orm_test.go` — 49 scenarios covering the full ORM data API through HTTP routes:
- **Schema management**: list tables, describe, create/delete, export, list ORM schemas, rename
- **CRUD**: insert (with auto-generated system fields), find, find-one, get-by, update by id, delete by id
- **Query ops**: WHERE filtering, ORDER+LIMIT, field selection, exists, count (with/without WHERE)
- **Pluck/distinct**: single-column extraction, unique values
- **Aggregate**: COUNT, SUM, GROUP BY
- **Bulk ops**: update-where, delete-where, upsert (insert + update paths)
- **Access policies**: set-policy (valid + invalid), role endpoint
- **Validation**: 11 error-case scenarios (missing table, missing columns, missing where, etc.)

**Bug fix**: `db.Upsert()` insert path now auto-generates values for `auto` columns (guid, datetime, date, time, integer) — same as `OrmInsert()`. Previously, upserting a new row into an ORM table with auto-guid keys silently failed because the NOT NULL guid column got no value. Fixed by calling `GetSchema()` before acquiring the write lock and applying the same auto-fill logic from `OrmInsert`.

**Warning fix**: removed unnecessary type arguments at `orm.go:287` (`orm.NewRepository[schema.Row]` → `orm.NewRepository`).

### Stale comments

- `internal/call/session.go:188,199` — Phase TODO markers replaced with descriptive comments about what's missing
- `internal/viewer/routes/call.go:409,431` — Phase TODO markers replaced with "Stub: not yet implemented"
- These are real stubs (loopback SDP/ICE not wired, track mute/disable state tracked but not applied to Pion)

### Viewer struct bloat

- `Cfg any` removed from `Viewer`, `MinimalViewer`, and `Deps` — was dead code (assigned but never read; routes load config fresh from `CfgPath`)
- Removed `Cfg:` assignments in peer.go, bridge.go, rendezvous.go
- Both `Viewer` and `Deps` fields grouped into semantic sections: Identity, Config & content, Storage, Networking, Core managers, Group-type managers, Avatar, Lua integration, Platform-specific

### Two chat packages renamed

- `internal/chat/` renamed to `internal/directchat/` (P2P + broadcast chat, persisted)
- `internal/group_types/chat/` stays as `chat` (group-bounded chat rooms)
- No more `chatType` alias needed — `group_types/chat` is now just `chat` everywhere
- `viewer.Viewer` field renamed: `Chat` → `DirectChat`
- `templateType` alias remains (Go stdlib `template` name collision, unavoidable)
- Chat system has 3 modes: direct (persisted, P2P), group rooms (group protocol), broadcast (ephemeral MQ, no manager — frontend-only over `chat.broadcast` topic)

### Error handling asymmetry in group_types/chat/events.go

- Removed stale nil guard from `publishLocal` — NopTransport handles nil-safety now
- Both `sendToPeer` and `publishLocal` now consistently rely on the Transport interface

### PeerTable unit tests

- Added `peers_test.go` with 13 tests covering Upsert (new, preserve local state, clear offline), Seed, SetReachable (success, fail streak, reset), MarkOffline, PruneStale (TTL, grace), Subscribe, Remove, Snapshot

### MQ Manager.Send integration test

- Added `internal/mq/send_test.go` — 7 tests using two in-process libp2p hosts with real MQ Managers
- Tests: DeliveredAndAcked, TopicSubscriberReceives, Bidirectional, InvalidPeerID, UnreachablePeer, MultipleMessages_SequenceIncreases, InboxBuffering_NoListener
- Covers the full Send→handleIncoming→ACK round-trip over the real wire protocol
- Discovery: `logMQEvent` publishes `log:mq` events to SSE listeners — tests must filter by topic to avoid matching log events

### Direct &Manager{} construction in tests

- `chat/handler_test.go` — refactored `testManager` to use `NewTestManager` from `testing.go`, added `testManagerOpts` for custom selfID/resolvePeer; `TestResolveMembersUsesPeerNameFallback` no longer duplicates Manager construction
- `datafed/handler_test.go` — `testManager` now delegates to `NewTestManager` instead of building `&Manager{}` directly
- `listen/queue_test.go` — created `testing.go` with `NewTestManager`, `SetTestGroup`, `SetTestQueue`; `TestQueuePersistence` uses these helpers
- `listen/events_test.go` `TestFlags` and `chat/handler_test.go` `TestFlags` left as `&Manager{}` — intentional zero-value tests (Flags() is a static return)
- `listen/queue_test.go` `TestQueuePersistenceNoStore` left as `&Manager{}` — intentionally tests nil-store behavior

### group/testing.go API change (naming consistency)

- `NewTestManager(db, selfID, opts...)` with `TestManagerOpts` — already documented in testing.md (line 78, 89, 115). All callers verified correct. Nothing left to do.

### viewer/routes file sizes (code structure)

- groups.go (461 lines), data.go (664 lines), call.go (452 lines) — moderate, not extreme. No file over 700 lines. Not worth splitting right now.

### repetitive JSON decode pattern (code structure)

- Only 4 occurrences of `json.NewDecoder(r.Body).Decode` and 4 of `json.NewEncoder(w).Encode` in routes. The codebase already uses `writeJSON`/`http.Error`/helpers extensively. The "20+" threshold for a typed handler helper is not met. No action needed.

### group.New() host.Host requirement (code structure)

- `New(h host.Host, ...)` for production, `NewTestManager(db, selfID, opts...)` for tests — intentional split. `New` needs host for `h.ID()` and `h.Connect()`. Already documented in testing.md. No action needed.

### Other completed work

- testpeer package created (bus.go, adapter.go, peer.go) with 10 tests
- group.NewTestManager updated to accept TestManagerOpts with MQ
- NopTransport added as default for test managers
- All field/param naming made consistent (mq field, transport param)
- shareddocs/internals/architecture.md and identity.md fully rewritten
- Fixed chat/handler_test.go — two direct Manager{} creations missing NopTransport (caused panic after nil guard removal)
