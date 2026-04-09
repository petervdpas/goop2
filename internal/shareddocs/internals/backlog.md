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
9. **Do the permanent tasks after every TODO.** After completing each TODO item, run the permanent tasks (bottom of this file) before moving to the next TODO. They never close.
10. **Commit nothing.** The user commits when ready.

---

## Test coverage (2026-04-06 baseline)

Run: `go test ./... -coverprofile=coverage.out -covermode=atomic`

| Package | Coverage | Notes |
|---------|----------|-------|
| app/shared | 100% | Done |
| ui/viewmodels | 100% | Done |
| config | 94.9% | Done |
| avatar | 96.2% | Done |
| orm/transformation | 90.2% | - |
| orm/schema | 86.6% | - |
| state | 83.2% | - |
| bridge | 82.5% | Done |
| testpeer | 82.8% | Test infra |
| orm | 82.3% | - |
| template (group_type) | 82.1% | - |
| sitetemplates | 81.1% | - |
| content | 74.9% | Done |
| files | 74.1% | - |
| mq | 73.8% | - |
| crypto | 77.3% | - |
| directchat | 72.4% | - |
| lua | 63.8% | - |
| gql | 64.2% | - |
| cluster | 56.9% | - |
| datafed | 53.1% | - |
| viewer | 48.1% | Done |
| chat (group_type) | 44.9% | - |
| util | 37.1% | - |
| rendezvous | 30.0% | Large package |
| ui/render | 29.6% | Done (template init limits) |
| group | 25.3% | - |
| call | 23.3% | Pion WebRTC |
| storage | 75.7% | Done |
| viewer/routes | 11.1% | Large package |
| app | 10.8% | Orchestration |
| p2p | 6.4% | Needs libp2p hosts |
| listen | 24.9% | Done (partial — host/client need libp2p) |
| app/modes | 0% | Pure orchestration |

### DONE: storage — 17.0% → 75.7% coverage

### TODO: util — 37.1% coverage

- `internal/util/` — shared helpers (DNS cache, ring buffer, file helpers, URL normalization, etc.)
- Pure utility functions, easy to test
- Files: `internal/util/`

### DONE: listen — 2.9% → 24.9% coverage

### TODO: group — 25.3% coverage

- `internal/group/` — group manager, host/client lifecycle, routing
- Core system — many code paths untested
- Focus: host create/close edge cases, routing logic, group state queries
- Files: `internal/group/`

### TODO: viewer/routes — 11.1% coverage

- `internal/viewer/routes/` — 80 tests exist but package is large (many route handlers)
- Focus on untested route handlers: peer routes, settings, editor, groups, call, listen, chat rooms
- Files: `internal/viewer/routes/`

### TODO: chat (group_type) — 44.9% coverage

- `internal/group_types/chat/` — chat room manager, message delivery, history
- Room lifecycle partially tested but many event handlers untested
- Files: `internal/group_types/chat/`

### TODO: datafed — 53.1% coverage

- `internal/group_types/datafed/` — data federation, contributions, sync
- Handler logic and sync flows need more coverage
- Files: `internal/group_types/datafed/`

### TODO: cluster — 56.9% coverage

- `internal/group_types/cluster/` — cluster compute, job dispatch
- Worker assignment and job lifecycle flows need coverage
- Files: `internal/group_types/cluster/`

### TODO: p2p — 6.4% coverage

- `internal/p2p/` — libp2p node, peer discovery, data exchange, relay, site file serving
- Needs real libp2p hosts — use `libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))` like existing tests in `circuit_stream_test.go` and `relay_lazy_test.go`
- Focus: data request/response handlers, site file fetch, peer connection helpers, topology
- Files: `internal/p2p/`

### TODO: call — 23.3% coverage

- `internal/call/` — Pion WebRTC call manager, SDP/ICE signaling, track management
- Pion has test helpers (`webrtc.NewAPI` with `SettingEngine`) — use those for session lifecycle
- Focus: session create/close, offer/answer exchange, track add/remove, call state queries
- Files: `internal/call/`

### TODO: rendezvous — 30.0% coverage

- `internal/rendezvous/` — server + client, large package
- Client WS state machine tested this session, but server handlers still low
- Focus: publish handler, peer state management, punch hints, entangle, admin panel, service proxies
- Files: `internal/rendezvous/`

### TODO: app — 10.8% coverage

- `internal/app/` — WaitTCP and setupMicroService tested, but run.go orchestration is untested
- Focus: runPeer step counting logic, NaCl key generation path, mode selection (rendezvous-only, bridge, peer)
- Needs config fixtures + temp dirs, no network required for the logic branches
- Files: `internal/app/run.go`

## BDD feature gaps

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

### TODO: Prezentia — zoomable canvas presentation template

Prezi-style presentation tool: content nodes (text, images) placed on a zoomable 2D canvas, with a path that defines the presentation order. Camera zooms/pans between nodes during playback. Single-author, not collaborative. Heavy use of site folder for image upload/delete.

**SDK extension:**

1. New `internal/sdk/goop-component-canvas.js` — reusable zoomable/pannable canvas (behaviour only, no CSS)
   - Pan (drag empty space), zoom (wheel/pinch), transform tracking
   - `animateTo(rect, duration)` — smooth camera transitions (the core of presentation playback)
   - Coordinate conversion: `toCanvas(screenX, screenY)` / `toScreen(canvasX, canvasY)`
   - `fitBounds(rect, padding)`, `getTransform()`, `setTransform()`
   - Free-form item dragging that accounts for zoom/pan transform
   - Reusable by any template wanting spatial/map/diagram views

**Template files:**

2. `internal/sitetemplates/prezentia/manifest.json` — name "Prezentia", category "content", icon, schemas: ["slides", "prezentia_config"]
3. `internal/sitetemplates/prezentia/schemas/slides.json` — columns: x, y, width, height, scale, rotation, type (text/image), content, position (path order), style_json
4. `internal/sitetemplates/prezentia/schemas/prezentia_config.json` — KV config (canvas bg, title, theme)
5. `internal/sitetemplates/prezentia/lua/functions/prezentia.lua` — CRUD for slides + config, follows blog.lua pattern with `goop.route()`
6. `internal/sitetemplates/prezentia/index.html` — editor + presenter UI, loads canvas SDK + site SDK + drag SDK
7. `internal/sitetemplates/prezentia/css/style.css` — all visual design (canvas, sidebar, nodes, presenter, responsive)
8. `internal/sitetemplates/prezentia/js/app.js` — editor: canvas interaction, node CRUD, image upload/delete via `Goop.site`, path editor sidebar (drag reorder via existing drag SDK), present button
9. `internal/sitetemplates/prezentia/js/present.js` — presentation engine: camera follows path via `canvas.animateTo()`, arrow key / click navigation, fullscreen
10. `internal/sitetemplates/prezentia/images/.keep`

**Registration:**

11. `internal/sitetemplates/embed.go:12` — add `all:prezentia` to the `//go:embed` directive

**Key design decisions:**
- Lua = control plane (slide CRUD, config), JS SDK = data plane (canvas, file ops)
- Images stored in `images/` via `Goop.site.upload/remove` — owner only
- Existing `Goop.drag.sortable` reused for path order sidebar
- No collaboration — owner edits, visitors see presentation
- `goop.config()` pattern for prezentia_config (same as blog_config)

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
