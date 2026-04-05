# Cleanup Backlog

## How to work with this file

You are reading this because the user asked you to improve the codebase. Follow these rules:

1. **Ask the user first:** "Want to add new items to the backlog, or shall I start working on the open TODOs?" Then proceed accordingly.
2. **Read the code first.** Every TODO has file paths. Read those files before changing anything.
3. **Fix it, then mark it DONE.** Add a one-line summary of what you did under the item.
4. **Add what you find.** Every file you read is a chance to spot new problems. Add them as new TODOs immediately — naming issues, dead code, missing tests, inconsistencies, anything that doesn't read like a story.
5. **Update the other internals docs.** If your fix changes how a system works, update the matching doc (architecture.md, identity.md, mq-internals.md, chat-internals.md, etc.). These docs are YOUR memory across sessions — stale docs waste the user's money.
6. **Run tests before declaring done.** `go test ./...` must pass. No exceptions.
7. **Don't ask the user what to prioritize.** The backlog IS the priority list. Work through it.
8. **Don't spawn agents for this work.** Read the files yourself. You need to understand what you're fixing.
9. **Do the permanent tasks** (bottom of this file) every session — they never close.
10. **Commit nothing.** The user commits when ready.

---

## Naming consistency

### DONE: mq.Transport interface and field naming

- `internal/mq/transport.go` — interface extracted, file renamed from sender.go
- All manager fields: `mq mq.Transport` (consistent)
- All constructor params: `transport mq.Transport` (declarative)
- Viewer routes keep `mqMgr *mq.Manager` (correct — they need the concrete type)
- `mq.NopTransport{}` default in test managers

### DONE: stale comments

- `internal/call/session.go:188,199` — Phase TODO markers replaced with descriptive comments about what's missing
- `internal/viewer/routes/call.go:409,431` — Phase TODO markers replaced with "Stub: not yet implemented"
- These are real stubs (loopback SDP/ICE not wired, track mute/disable state tracked but not applied to Pion)

### TODO: Viewer struct bloat

- `internal/viewer/viewer.go` — `Viewer` struct has 20+ fields with inconsistent commenting
- `Cfg any` with comment "avoid import cycle" — investigate if the cycle still exists or if this can be typed
- Consider grouping related fields (all the managers, all the config, all the identity funcs)

### DONE: two chat packages renamed

- `internal/chat/` renamed to `internal/directchat/` (P2P + broadcast chat, persisted)
- `internal/group_types/chat/` stays as `chat` (group-bounded chat rooms)
- No more `chatType` alias needed — `group_types/chat` is now just `chat` everywhere
- `viewer.Viewer` field renamed: `Chat` → `DirectChat`
- `templateType` alias remains (Go stdlib `template` name collision, unavoidable)
- Chat system has 3 modes: direct (persisted, P2P), group rooms (group protocol), broadcast (ephemeral MQ, no manager — frontend-only over `chat.broadcast` topic)

### TODO: direct &Manager{} construction in tests

- `group_types/chat/handler_test.go` — two places create `&Manager{}` directly (fixed for mq, but pattern remains)
- `group_types/listen/queue_test.go` and `events_test.go` — same pattern
- `group_types/datafed/handler_test.go` — same pattern
- These bypass `NewTestManager` and break when fields gain non-nil requirements
- Fix: either use `NewTestManager` consistently, or at minimum ensure `mq: mq.NopTransport{}` is always set

### TODO: group/testing.go API change

- Changed from `NewTestManager(db, selfID, resolvePeer...)` to `NewTestManager(db, selfID, opts...)`
- All existing callers updated but the opts pattern means you need to know about `TestManagerOpts` struct
- This is fine but should be documented in the test helpers table (now done in testing.md)

## Code structure

### TODO: viewer/routes file sizes

- `internal/viewer/routes/groups.go` — large file, handles hosted + joined + subscriptions + SSE
- `internal/viewer/routes/data.go` — large file, handles tables + CRUD + ORM queries
- `internal/viewer/routes/call.go` — large file, handles all call modes + loopback + media streaming
- Not broken but hard to navigate. Consider splitting the largest ones (groups.go → groups_hosted.go + groups_client.go + groups_sse.go)

### TODO: repetitive JSON decode pattern in routes

- Many route handlers repeat: `json.NewDecoder(r.Body).Decode(&req)` → check error → process → `json.NewEncoder(w).Encode(resp)`
- Not a bug but reads as boilerplate. Consider a typed handler helper if the pattern appears 20+ times

### DONE: error handling asymmetry in group_types/chat/events.go

- Removed stale nil guard from `publishLocal` — NopTransport handles nil-safety now
- Both `sendToPeer` and `publishLocal` now consistently rely on the Transport interface

### TODO: `group.New()` requires host.Host but TestPeer passes nil

- `group.New(h host.Host, db, transport, resolvePeer)` uses `h.ID().String()` for selfID
- TestPeer uses `group.NewTestManager(db, id, opts)` which skips this
- But `group.New` also calls `h.Connect()` in `JoinRemoteGroup` — this means production `New` fundamentally needs a host
- This is correct. The split between `New` (production, needs host) and `NewTestManager` (testing, no host) is intentional

## Test gaps

### DONE: PeerTable unit tests

- Added `peers_test.go` with 13 tests covering Upsert (new, preserve local state, clear offline), Seed, SetReachable (success, fail streak, reset), MarkOffline, PruneStale (TTL, grace), Subscribe, Remove, Snapshot

### TODO: MQ Manager.Send test

- Currently untestable without libp2p (Send opens a stream)
- The TestBus/MQAdapter tests cover the Transport interface but not the real Manager
- Consider: a test that creates two libp2p hosts in-process and tests real Send/receive

### TODO: viewer route tests

- Most routes have zero unit tests
- BDD tests in `tests/chatrooms/` cover chat room routes but nothing else
- Low priority but fragile — route changes can break silently

### TODO: rendezvous server WebSocket reconnection

- `client.go` ConnectWebSocket has exponential backoff + SSE fallback
- No test for the reconnection logic or the SSE→WS upgrade probe
- Hard to test (needs a real server), but the state machine could be unit tested

## Recently completed

- 2026-04-06: mq.Transport interface extracted, all managers updated
- 2026-04-06: testpeer package created (bus.go, adapter.go, peer.go) with 10 tests
- 2026-04-06: group.NewTestManager updated to accept TestManagerOpts with MQ
- 2026-04-06: NopTransport added as default for test managers
- 2026-04-06: All field/param naming made consistent (mq field, transport param)
- 2026-04-06: shareddocs/internals/architecture.md and identity.md fully rewritten
- 2026-04-06: Stale TODO/Phase comments cleaned up in call.go and session.go
- 2026-04-06: chat/events.go nil guard asymmetry fixed
- 2026-04-06: PeerTable unit tests added (13 tests in peers_test.go)
- 2026-04-06: cleanup-backlog.md created as living backlog
- 2026-04-06: Fixed chat/handler_test.go — two direct Manager{} creations missing NopTransport (caused panic after nil guard removal)
- 2026-04-06: Renamed internal/chat → internal/directchat, dropped chatType alias everywhere

## Permanent tasks

These never close. Every session must do them.

### Maintain internals docs

1. **Read cleanup-backlog.md first** to know what needs work
2. **Update it while working** — mark DONE, add new findings
3. **Keep state docs accurate** — when code changes, update the matching internals doc (architecture.md, identity.md, testing.md, mq-internals.md, etc.)
4. **Add new issues as discovered** — every file read is an opportunity to spot problems

The internals docs exist so future sessions don't waste tokens re-reading code. They must reflect current reality, not a past snapshot.

### Verify OpenAPI annotations

`internal/viewer/routes/openapi_annotations.go` contains swaggo annotation stubs that mirror the real route handlers. When routes change (new endpoints, changed request/response shapes, renamed fields), the annotations must be updated to match. Every session that touches route handlers should:

1. Check if the affected endpoint has an annotation stub in `openapi_annotations.go`
2. If yes, verify the stub's request/response types match the actual handler
3. If a new endpoint was added, add a corresponding annotation stub
4. Run `go generate` if annotations changed to regenerate `docs/`
