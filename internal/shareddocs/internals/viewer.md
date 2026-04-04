# Viewer & HTTP Layer

## Deps struct

`internal/viewer/routes/register.go`

Central dependency container passed to all route registration functions:

| Field | Type | Purpose |
| -- | -- | -- |
| `Node` | `*p2p.Node` | P2P networking (nil in rendezvous-only mode) |
| `SelfLabel` | `func() string` | Local peer's display name |
| `SelfEmail` | `func() string` | Local peer's email |
| `Peers` | `*state.PeerTable` | In-memory peer state |
| `CfgPath` | `string` | Path to `goop.json` |
| `Cfg` | `any` | Config struct (any to avoid import cycle) |
| `Logs` | `Logs` | Log buffer interface |
| `Content` | `*content.Store` | Site content file store |
| `BaseURL` | `string` | Canonical base URL for the viewer |
| `DB` | `*storage.DB` | SQLite database |
| `AvatarStore` | `*avatar.Store` | Avatar file storage |
| `AvatarCache` | `*avatar.Cache` | Avatar cache |
| `PeerDir` | `string` | Root directory for this peer's data |
| `RVClients` | `[]*rendezvous.Client` | Rendezvous server connections |
| `BridgeURL` | `string` | Wails bridge URL (empty when not in Wails) |
| `RendezvousOnly` | `bool` | Rendezvous-only mode (limited nav) |
| `RendezvousURL` | `string` | Rendezvous server URL |
| `DocsStore` | `*files.Store` | Document sharing storage |
| `GroupManager` | `*group.Manager` | Group protocol manager |
| `TemplateHandler` | `*template.Handler` | Template lifecycle handler |
| `EnsureLua` | `func()` | Start Lua engine and rescan functions |
| `LuaCall` | `func(ctx, function, params)` | Invoke a Lua data function |

Assembled in `viewer.go` from the `Viewer` struct, which is assembled in `app/modes/peer.go`.

## Route registration

`internal/viewer/routes/register.go`

- `handleGet` / `handlePost` helpers: JSON decode for POST bodies, standard error responses
- CSRF token: generated once in `Register()`, passed to templates for form validation
- All routes registered via `Register(mux, deps)` which calls domain-specific functions:
  - `registerHomeRoutes`, `registerPeerRoutes`, `registerSelfRoutes`
  - `registerEditorRoutes`, `registerSettingsRoutes`
  - `registerSimplePages` (logs, database, groups tabs)
  - `RegisterChatRooms`, `RegisterGroups`, `registerTemplateRoutes`, etc.

- `RegisterMinimal(mux, deps)` — used in rendezvous-only mode (self/settings and logs only)

## Template rendering

Go `html/template` with `render.InitTemplates()` at startup.

Templates live in `internal/ui/templates/*.html`. The layout system uses `layout.html` as the base wrapper with `{{template .ContentTmpl .}}` for page-specific content.

`BaseVM` (in `internal/ui/viewmodels/base.go`) carries shared data to all templates: title, active nav, self info, base URL, debug flag, theme, OS, run ID, split pane prefs.

## UI assets loading

`internal/ui/assets/app.js` loads all JavaScript files sequentially in three groups:

1. **sharedFiles** — core infrastructure: `core.js`, `api.js`, `mq/base.js`, `mq/topics.js`, `mq/peers.js`, `select.js`, `panel.js`, `layout.js`, `groups.js`, `dialogs.js`, etc.
2. **callFiles** — call layer: `call.js`, `call-ui.js`
3. **pageFiles** — page-specific JS: `pages/peers.js`, `pages/editor.js`, `pages/database.js`, `pages/logs.js`, `pages/groups.js`, `pages/self.js`, etc.

Each page JS file self-initializes by checking for its DOM elements (e.g., `if (!document.getElementById("peers-list")) return;`).

## Admin UI vs SDK

Two separate JavaScript ecosystems in the same app:

**Admin UI** (`internal/ui/assets/js/`):

- Core modules: `Goop.core`, `Goop.api`, `Goop.panel`, `Goop.groups`, `Goop.dialog`
- Loaded by `app.js` into the viewer pages
- Full access to all API endpoints

**SDK** (`internal/sdk/goop-*.js`):

- Served at `/sdk/` for template viewer pages
- Modules: `Goop.data`, `Goop.ui`, `Goop.group`, `Goop.template`, `Goop.mq`, `Goop.chatroom`, `Goop.peers`, `Goop.chat`, `Goop.call`, `Goop.realtime`
- Fully standalone — no dependency on admin UI `core.js`
- Templates include SDK scripts via `<script src="/sdk/goop-data.js"></script>`

## OpenAPI annotations

`internal/viewer/routes/openapi_annotations.go`

Every HTTP endpoint has a matching Swagger stub function with `@Summary`, `@Tags`, `@Accept`, `@Produce`, `@Param`, `@Success`, `@Failure`, `@Router` annotations.

Request/response types are defined as unexported structs in the same file. These structs exist solely for Swagger documentation and are not used in application code.
