# Viewer & HTTP Layer

## Deps struct

`internal/viewer/routes/register.go`

Central dependency container passed to all route registration functions:

Grouped into semantic sections:

**Identity**
| Field | Type | Purpose |
| -- | -- | -- |
| `Node` | `*p2p.Node` | P2P networking (nil in rendezvous-only mode) |
| `SelfLabel` | `func() string` | Local peer's display name |
| `SelfEmail` | `func() string` | Local peer's email |
| `Peers` | `*state.PeerTable` | In-memory peer state |
| `ResolvePeer` | `func(string) state.PeerIdentityPayload` | Resolve peer ID to identity |

**Config & content**
| Field | Type | Purpose |
| -- | -- | -- |
| `CfgPath` | `string` | Path to `goop.json` |
| `PeerDir` | `string` | Root directory for this peer's data |
| `Content` | `*content.Store` | Site content file store |
| `Logs` | `Logs` | Log buffer interface |
| `BaseURL` | `string` | Canonical base URL for the viewer |

**Storage**
| Field | Type | Purpose |
| -- | -- | -- |
| `DB` | `*storage.DB` | SQLite database |

**Networking**
| Field | Type | Purpose |
| -- | -- | -- |
| `BridgeURL` | `string` | Wails bridge URL (empty when not in Wails) |
| `RVClients` | `[]*rendezvous.Client` | Rendezvous server connections |
| `RendezvousOnly` | `bool` | Rendezvous-only mode (limited nav) |
| `RendezvousURL` | `string` | Rendezvous server URL |
| `TopologyFunc` | `func() any` | Override for topology endpoint |

**Group managers**
| Field | Type | Purpose |
| -- | -- | -- |
| `GroupManager` | `*group.Manager` | Group protocol manager |
| `DocsStore` | `*files.Store` | Document sharing storage |
| `TemplateHandler` | `*template.Handler` | Template lifecycle handler |

**Avatar**
| Field | Type | Purpose |
| -- | -- | -- |
| `AvatarStore` | `*avatar.Store` | Avatar file storage |
| `AvatarCache` | `*avatar.Cache` | Avatar cache |

**Lua integration**
| Field | Type | Purpose |
| -- | -- | -- |
| `EnsureLua` | `func()` | Start Lua engine and rescan functions |
| `LuaCall` | `func(ctx, function, params)` | Invoke a Lua data function |

Assembled in `viewer.go` from the `Viewer` struct, which is assembled in `app/modes/peer.go`.

## Route registration

`internal/viewer/routes/register.go`

Route helpers in `helpers.go` eliminate boilerplate:

| Helper | What it does |
| -- | -- |
| `handleGet(mux, path, fn)` | Registers GET handler with method check |
| `handlePost[T](mux, path, fn)` | Generic typed POST: method check + `decodeJSON` into `T`, then calls `fn(w, r, T)` |
| `handlePostAction(mux, path, fn)` | POST with method check but no body decoding |
| `handleFormPost(mux, path, csrf, fn)` | POST with CSRF + localhost check + form parsing |
| `writeJSON(w, v)` | Sets Content-Type + encodes JSON response |
| `decodeJSON(w, r, v)` | Decodes JSON body, sends 400 on failure |
| `requireMethod(w, r, method)` | Method guard, sends 405 on mismatch |
| `requireLocal(w, r)` | Localhost guard, sends 403 on mismatch |

Only 4 route handlers still use raw `json.NewDecoder` — the rest use `handlePost[T]` or `decodeJSON`.

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
