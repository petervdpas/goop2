# Wails Desktop Layer

## Overview

The Wails desktop app (`app.go` + `main.go`) provides the native launcher window and bridges to the internal viewer.

## App struct

```go
type App struct {
    ctx, cancel          // Wails lifecycle context
    peerDir, cfgPath     string
    peerName             string
    started              bool
    viewerURL            string
    isRendezvousOnly     bool
    bridgeURL            string
}
```

## Launcher flow

1. **Window created**: Wails opens with title `"Goop² · ephemeral web · v{appVersion}"`
2. **Theme loaded**: `App.GetTheme()` reads from `data/ui.json`
3. **Peer list**: `App.ListPeers()` scans `./peers/` for directories with `goop.json`
4. **Create peer**: `App.CreatePeer(name, siteFolder)` creates peer directory, default config, copies site files
5. **Start peer**: `App.StartPeer(peerName)` loads config, calls `goopapp.Run()` in a goroutine
6. **Viewer ready**: Waits for TCP listener (30s timeout), emits `startup:done` event
7. **Frontend navigates**: Replaces launcher content with viewer at `viewerURL`

## Wails bindings (exposed to frontend JS)

Called via `window.go.main.App.MethodName()`:

| Method | Purpose |
| -- | -- |
| `GetTheme()` | Read theme from `data/ui.json` |
| `SetTheme(theme)` | Write theme to `data/ui.json` |
| `GetBridgeURL()` | Bridge URL for native dialogs |
| `GetVersion()` | Build version (`appVersion` from ldflags) |
| `OpenInBrowser(url)` | Open URL in system browser |
| `SelectSiteFolder()` | Native folder picker dialog |
| `ListPeers()` | Scan peers directory, return `[]PeerInfo` |
| `CreatePeer(name, siteFolder)` | Create new peer directory + config |
| `DeletePeer(name)` | Remove peer directory |
| `StartPeer(peerName)` | Start peer node + viewer |
| `GetViewerURL()` | Internal viewer URL |
| `GetStatus()` | Started, peerName, viewerURL, rendezvousOnly |

## Version

`appVersion` is set at build time via:
```
-ldflags "-X main.appVersion=2.4.x"
```

Default: `"dev"`. Shown in:

- Window title bar (always)
- Announced to peers as `GoopClientVersion` in presence messages

## Theme persistence

Stored in `data/ui.json` (relative to working directory, shared across peers):
```json
{"theme": "dark"}
```

Read by both the launcher (Wails) and the viewer (Go templates).

## Frontend

`frontend/src/main.js` — Vite-bundled SPA:

1. Sets brand icon
2. Wires theme toggle
3. Waits for Wails runtime
4. Checks if peer already started → navigate directly to viewer
5. Otherwise renders launcher UI (peer list, create, start/delete)

## Startup events

The App emits Wails events during peer startup:

- `startup:progress` — `{step, total, label}` for progress bar
- `startup:done` — viewer URL available, frontend navigates
- `startup:error` — startup failed
