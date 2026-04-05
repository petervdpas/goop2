# Backlog History

Completed items moved from `backlog.md`.

---

## 2026-04-06

### mq.Transport interface and field naming

- `internal/mq/transport.go` — interface extracted, file renamed from sender.go
- All manager fields: `mq mq.Transport` (consistent)
- All constructor params: `transport mq.Transport` (declarative)
- Viewer routes keep `mqMgr *mq.Manager` (correct — they need the concrete type)
- `mq.NopTransport{}` default in test managers

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

### Other completed work

- testpeer package created (bus.go, adapter.go, peer.go) with 10 tests
- group.NewTestManager updated to accept TestManagerOpts with MQ
- NopTransport added as default for test managers
- All field/param naming made consistent (mq field, transport param)
- shareddocs/internals/architecture.md and identity.md fully rewritten
- Fixed chat/handler_test.go — two direct Manager{} creations missing NopTransport (caused panic after nil guard removal)
