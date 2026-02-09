# Plan: Public Site Bridge

## Summary

Add a `public_addr` setting to the viewer that starts a second, minimal HTTP server exposing only the peer's site to the outside world. Visitors with a regular browser can view the site and read data (blog posts, leaderboards, pinboards) but cannot write, execute Lua, or access any admin functionality.

Templates opt in to public access via a `"public": true` flag in their `manifest.json`.

## Why

A Goop2 peer already serves sites over HTTP at `/p/<peer-id>/`. But the viewer exposes everything on one port: settings, editor, file manager, database admin. Binding to `0.0.0.0` would be a security problem.

The public bridge solves this by offering a **safe, separate port** that only serves site content. This enables:

- A teacher running a quiz — students open it on their phones
- A community corkboard on a Raspberry Pi
- A game night with friends — just share the link
- A read-only blog or portfolio accessible to anyone
- An intranet dashboard on the office network

## Config changes

### `internal/config/config.go` — Add field to Viewer struct

```go
type Viewer struct {
    HTTPAddr    string `json:"http_addr"`
    PublicAddr  string `json:"public_addr"`  // NEW — empty = disabled
    Debug       bool   `json:"debug"`
    // ...existing fields...
}
```

Default: `""` (disabled). Example: `":8888"` or `"0.0.0.0:8888"`.

### Template manifest — Add `public` flag

Both `TemplateMeta` (`internal/sitetemplates/embed.go`) and `StoreMeta` (`creditapi/creditapi.go`) get a new field:

```go
Public bool `json:"public,omitempty"`
```

Templates that work well as public read-only sites set `"public": true` in their `manifest.json`:

```json
{
  "name": "Blog",
  "public": true,
  "tables": {
    "posts": { "insert_policy": "owner" }
  }
}
```

Templates without `"public": true` are not served on the bridge. The bridge returns a friendly message like "This site requires a Goop2 peer to view."

## What the bridge serves

| Route | Method | Purpose |
|-------|--------|---------|
| `/` | GET | Redirect to `/p/<self-peer-id>/` |
| `/p/<peer-id>/*` | GET | Static site files (HTML, CSS, JS, images) |
| `/assets/*` | GET | Viewer client assets (`goop-data.js`, `goop-forms.js`) |
| `/api/data/tables` | GET | List tables (read-only) |
| `/api/data/query` | POST | Query rows (read-only) |
| `/api/data/tables/describe` | POST | Table schema (read-only) |
| `/healthz` | GET | Health check for reverse proxies |

## What the bridge blocks

Everything else. Specifically:

| Blocked | Why |
|---------|-----|
| `/api/data/insert`, `update`, `delete` | Write operations need peer identity |
| `/api/data/tables/create`, `delete`, `add-column`, etc. | Schema modifications |
| `/api/data/lua/*` | Lua execution expects peer context |
| `/self`, `/settings`, `/settings/save` | Admin/settings UI |
| `/edit`, `/api/editor/*` | Code editor |
| `/api/site/upload`, `delete` | File management |
| `/database` | Database admin UI |
| `/peers`, `/api/peers` | Peer list |
| `/chat`, `/api/chat/*` | Chat (peer-only) |
| `/groups`, `/api/groups/*` | Group management |
| `/logs`, `/api/logs/*` | Server logs |
| `/docs`, `/api/docs/*` | File sharing |

## How `goop-data.js` works on the bridge

The client library already detects context from the URL path:

```js
var apiBase = "/api/data";
var m = window.location.pathname.match(/^\/p\/([^/]+)\//);
if (m) {
    apiBase = "/api/p/" + m[1] + "/data";
}
```

On the public bridge, the URL is `/p/<peer-id>/...`, but since the bridge only serves the peer's **own** site, the data API calls should go to `/api/data/` (local database), not through the P2P proxy. The bridge can either:

- **Option A**: Redirect `/api/p/<self-id>/data/*` to the local data handler (transparent)
- **Option B**: Serve a modified `goop-data.js` that always uses `/api/data/` on the bridge

Option A is cleaner — no template changes needed, the bridge just handles both paths.

## Implementation

### Files to modify

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `PublicAddr` to `Viewer` struct |
| `creditapi/creditapi.go` | Add `Public bool` to `StoreMeta` |
| `internal/sitetemplates/embed.go` | Add `Public bool` to `TemplateMeta` |
| `internal/viewer/viewer.go` | Add `StartPublic(addr string, v Viewer) error` function |
| `internal/app/run.go` | Launch `StartPublic` when `PublicAddr` is configured |
| `internal/viewer/routes/settings.go` | Save `public_addr` from form |
| `internal/ui/templates/self.html` | Add "Public site address" field in Network section |
| `internal/rendezvous/docs/04-configuration.md` | Document `public_addr` |
| `internal/rendezvous/docs/08-advanced.md` | Update "Exposing your site" section |
| Template manifests (blog, arcade, etc.) | Add `"public": true` where appropriate |

### `StartPublic()` — The bridge server

New function in `internal/viewer/viewer.go`:

```go
func StartPublic(addr string, v Viewer) error {
    mux := http.NewServeMux()

    // Check if the active template supports public access.
    // If not, serve a "not available publicly" page on all routes.

    // Static site files
    mux.HandleFunc("/p/", proxyPeerSite(v))

    // Redirect root to own site
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            http.NotFound(w, r)
            return
        }
        http.Redirect(w, r, "/p/"+v.Node.ID()+"/", http.StatusFound)
    })

    // Client assets needed by templates
    mux.Handle("/assets/", http.StripPrefix("/assets/",
        noCache(viewerassets.Handler()),
    ))

    // Read-only data API (local database)
    mux.HandleFunc("/api/data/", publicDataHandler(v.DB, v.Node.ID()))

    // Also handle /api/p/<self>/data/ → same read-only handler
    // (because goop-data.js generates these paths)
    mux.HandleFunc("/api/p/", publicDataProxyHandler(v.DB, v.Node.ID()))

    // Health check
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("ok"))
    })

    return http.ListenAndServe(addr, mux)
}
```

### `publicDataHandler()` — Read-only data

A new handler that only allows read operations:

```go
func publicDataHandler(db *storage.DB, selfID string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        path := strings.TrimPrefix(r.URL.Path, "/api/data/")
        switch path {
        case "tables":
            // allow — list tables
        case "query":
            // allow — read rows
        case "tables/describe":
            // allow — schema info
        default:
            http.Error(w, "forbidden", http.StatusForbidden)
        }
    }
}
```

### `publicDataProxyHandler()` — Redirect self-proxy to local

When `goop-data.js` generates `/api/p/<self-id>/data/query`, the bridge translates this to a local read-only query (same as above). Requests for other peer IDs are rejected — the bridge only serves the local peer's site.

### Template public flag check

On startup, `StartPublic` reads the active template's `manifest.json` from the site directory. If `"public"` is not `true`, the bridge serves a static page explaining the site isn't publicly available.

## Settings UI

In the **Network** section of `self.html`, add after the mDNS field:

```html
<div class="field">
  <label>Public site address</label>
  <input name="viewer_public_addr"
         value="{{.Cfg.Viewer.PublicAddr}}"
         placeholder=":8888">
  <div class="hint">
    Expose your site on a separate port for public web access (read-only).
    Only serves site content — no admin, editor, or write access.
    Leave empty to disable.
  </div>
</div>
```

## Which templates get `"public": true`

| Template | Public? | Reason |
|----------|---------|--------|
| Blog | Yes | Read-only feed, visitors just read posts |
| Corkboard | Yes | Read-only pinboard, visitors browse notes |
| Photobook | Yes | Read-only gallery |
| Arcade (Space Invaders) | Yes | Self-contained JS game, scores are read-only view |
| Quiz | No | Needs peer identity for scoring and answers |
| Chess | No | Needs Lua for move validation, needs peer identity |
| Kanban | No | Collaborative editing needs peer identity |
| Clubhouse | No | Real-time chat needs peer identity |
| Tic-Tac-Toe | No | Needs Lua and peer identity |
| Enquete | No | Survey responses need peer identity |

## Reverse proxy example

```
mysite.example.com {
    reverse_proxy localhost:8888
}
```

With a redirect rule for cleaner URLs:

```
mysite.example.com {
    redir / /p/{$PEER_ID}/ permanent
    reverse_proxy localhost:8888
}
```

## Future: Gateway microservice

The `public_addr` bridge works for peers that can open a port (home server, VPS, Pi). For peers behind strict NAT, a future **gateway microservice** could run alongside the rendezvous server with its own libp2p node, tunneling HTTP requests to peers over P2P streams. This is a larger effort (essentially ngrok for Goop2) and is tracked separately.

## Implementation order

1. Add `PublicAddr` to config struct + `Public` to manifest structs
2. Create `StartPublic()` + read-only data handlers in viewer
3. Wire up in `app/run.go`
4. Add UI field in Network settings + save handler
5. Set `"public": true` in appropriate template manifests
6. Update docs (configuration + advanced)
7. Build + test
