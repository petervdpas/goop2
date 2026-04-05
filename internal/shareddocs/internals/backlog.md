# Backlog

## How to work with this file

You are reading this because the user asked you to improve the codebase. Follow these rules:

1. **Ask the user first:** "Want to add new items to the backlog, or shall I start working on the open TODOs?" Then proceed accordingly.
2. **Read the code first.** Every TODO has file paths. Read those files before changing anything.
3. **Fix it, then mark it DONE.** Move it to `backlog-history.md` with a one-line summary of what you did.
4. **Add what you find.** Every file you read is a chance to spot new problems. Add them as new TODOs immediately — naming issues, dead code, missing tests, inconsistencies, anything that doesn't read like a story.
5. **Update the other internals docs.** If your fix changes how a system works, update the matching doc (architecture.md, identity.md, mq-internals.md, chat-internals.md, etc.). These docs are YOUR memory across sessions — stale docs waste the user's money.
6. **Run tests before declaring done.** `go test ./...` must pass. No exceptions.
7. **Don't ask the user what to prioritize.** The backlog IS the priority list. Work through it.
8. **Don't spawn agents for this work.** Read the files yourself. You need to understand what you're fixing.
9. **Do the permanent tasks** (bottom of this file) every session — they never close.
10. **Commit nothing.** The user commits when ready.

---

## Naming consistency

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

### TODO: `group.New()` requires host.Host but TestPeer passes nil

- `group.New(h host.Host, db, transport, resolvePeer)` uses `h.ID().String()` for selfID
- TestPeer uses `group.NewTestManager(db, id, opts)` which skips this
- But `group.New` also calls `h.Connect()` in `JoinRemoteGroup` — this means production `New` fundamentally needs a host
- This is correct. The split between `New` (production, needs host) and `NewTestManager` (testing, no host) is intentional

## Test gaps

### TODO: viewer route tests

- Most routes have zero unit tests
- BDD tests in `tests/chatrooms/` cover chat room routes but nothing else
- Low priority but fragile — route changes can break silently

### TODO: rendezvous server WebSocket reconnection

- `client.go` ConnectWebSocket has exponential backoff + SSE fallback
- No test for the reconnection logic or the SSE→WS upgrade probe
- Hard to test (needs a real server), but the state machine could be unit tested

## Features

### TODO: Template README.md

Every template should have a `README.md` next to `manifest.json`, editable by the template author through the viewer UI.

**Current state:**
- `embed.go:75` — `SiteFiles()` skips `manifest.json` and `schema.sql` but knows nothing about `README.md`
- `templates.go` apply flows (built-in :128, local :243, store :355) — all separate `manifest.json` and `schema.sql` from site files, `README.md` not handled
- `templates.go:407` — `GET /api/template/settings` returns manifest from `_meta["template_manifest"]`, no readme field
- `templates.html` — gallery cards show `.Description` (one-liner from manifest), no readme display
- Local/store apply both use `readLocalTemplateDir` / `extractTarGz` which return all files as `map[string][]byte`

**What needs to happen:**
1. `embed.go:75` — add `README.md` to `SiteFiles()` exclusion list (don't copy to site dir)
2. `templates.go` apply flows — extract `README.md` from file map, store as `_meta["template_readme"]`
3. `templates.go` local + store apply switches (lines 244, 356) — add `case "README.md"` next to `manifest.json`
4. `GET /api/template/settings` — include readme content in response
5. New `POST /api/template/readme` — save edited markdown back to `_meta["template_readme"]`
6. `internal/sitetemplates/*/README.md` — write one per built-in template
7. `TemplateMeta` struct does NOT need a readme field — readme is content, not metadata
8. Viewer UI: add editor (textarea) for markdown in the template settings area of `self.html`

## Permanent tasks

These never close. Every session must do them.

### Maintain internals docs

1. **Read backlog.md first** to know what needs work
2. **Update it while working** — move DONE items to `backlog-history.md`, add new findings
3. **Keep state docs accurate** — when code changes, update the matching internals doc (architecture.md, identity.md, testing.md, mq-internals.md, etc.)
4. **Add new issues as discovered** — every file read is an opportunity to spot problems

The internals docs exist so future sessions don't waste tokens re-reading code. They must reflect current reality, not a past snapshot.

### Verify OpenAPI annotations

`internal/viewer/routes/openapi_annotations.go` contains swaggo annotation stubs that mirror the real route handlers. When routes change (new endpoints, changed request/response shapes, renamed fields), the annotations must be updated to match. Every session that touches route handlers should:

1. Check if the affected endpoint has an annotation stub in `openapi_annotations.go`
2. If yes, verify the stub's request/response types match the actual handler
3. If a new endpoint was added, add a corresponding annotation stub
4. Run `go generate` if annotations changed to regenerate `docs/`
