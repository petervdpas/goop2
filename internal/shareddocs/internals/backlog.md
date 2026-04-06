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

## Missing test files

Packages with logic but zero test files (`go test` reports `[no test files]`).

### TODO: internal/config — no tests

- `config.go` has `Default()`, `Validate()`, and the `Presence` struct
- Validation rules and default values are prime unit test targets
- Risk: config changes silently break startup if Validate() logic drifts

### TODO: internal/avatar — no tests

- `avatar.go` + `cache.go` — avatar generation and caching
- Pure logic, easy to test: generate deterministic avatar, verify cache hit/miss

### TODO: internal/content — no tests

- `store.go` — content storage
- Data handling logic should have basic CRUD coverage

### TODO: internal/viewer — no tests

- `contenttype.go`, `logbuf.go`, `nocache.go`, `proxy.go`, `viewer.go`
- `logbuf.go` (ring buffer) and `contenttype.go` (MIME detection) are pure functions, trivial to test
- `proxy.go` could use an httptest-based test

### TODO: internal/ui/render — no tests

- `highlight.go` + `templates.go` — syntax highlighting and template rendering helpers
- `highlight.go` is pure transformation, easy to test

### TODO: internal/ui/viewmodels — no tests

- 8 files building view models (self, peer, peers, templates, settings, etc.)
- These are struct builders — test that fields are populated correctly from input data

### TODO: internal/bridge — no tests

- `client.go` — bridge client logic
- Network-dependent but the state machine / retry logic could be unit tested

### TODO: internal/app — no tests

- `helpers.go`, `services.go`, `timings.go`, `run.go` — app wiring and startup
- Mostly orchestration; `helpers.go` and `timings.go` may have testable pure functions

### TODO: internal/app/modes — no tests

- `bridge.go`, `peer.go`, `rendezvous.go`, `signaler.go` — run mode setup
- Heavy on wiring, low unit-test value unless logic is extracted

### TODO: internal/app/shared — no tests

- `opts.go` — shared options struct
- Likely just a struct definition; skip unless it has validation logic

## BDD feature gaps

### TODO: ORM operations feature

- Core data API (`goop.orm()` / `db.orm()`) has zero BDD coverage
- Unit tests exist in `internal/orm/orm_test.go` but no end-to-end feature file
- Should cover: create table via schema, insert/update/delete rows, query ops (find, find_one, pluck, count, distinct, aggregate), upsert, update_where, delete_where
- Access policies (owner-only, role-based) should be tested as scenarios
- Files: `internal/orm/`, `internal/viewer/routes/data.go`

### TODO: Lua sandbox feature

- Template Lua scripting has zero BDD coverage
- Unit tests exist (`internal/lua/lua_test.go`, `orm_test.go`, `blog_test.go`, `group_test.go`, `template_test.go`) but no feature file
- Should cover: `goop.orm()` from Lua, seed execution, rate limiting, memory limits
- Key risk: Lua↔Go boundary bugs (e.g. the empty table `{}` → `[]` fix) only caught by unit tests
- Files: `internal/lua/`

### TODO: Direct chat feature

- Direct chat subsystem has zero BDD coverage
- MQ feature tests cover topic routing but not actual message storage/retrieval
- Unit tests exist in `internal/directchat/manager_test.go`
- Should cover: send message, retrieve history, chat list, message ordering
- Files: `internal/directchat/`, `internal/viewer/routes/chat.go`

### TODO: Data proxy (P2P ORM) feature

- P2P data exchange has zero BDD coverage
- When a joiner queries a host's ORM table, requests go through the data proxy
- Should cover: remote query, remote insert (if allowed by access policy), access denial for unauthorized ops
- Hard to test (needs two peers) but the proxy logic could use an httptest-based BDD suite
- Files: `internal/viewer/routes/data_proxy.go`, `internal/p2p/data.go`

### TODO: Template lifecycle feature

- Template apply/remove/settings has zero BDD coverage
- Should cover: apply built-in template, apply local template, remove template (drops tables), template settings API
- The manifest/schema/site-file separation logic is complex and only tested manually
- Files: `internal/viewer/routes/templates.go`, `internal/sitetemplates/`

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
