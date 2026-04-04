# Avatar & Content Store Internals

## Avatar system

### Store (`internal/avatar/avatar.go`)

Manages the local peer's avatar file:

- File: `<peerDir>/avatar.png`
- Hash: SHA-256, truncated to 16 hex chars — used for cache invalidation
- Hash is cached in memory, recomputed on Write/Delete
- Thread-safe (sync.RWMutex)

| Method | Purpose |
| -- | -- |
| `Hash()` | Current avatar hash (empty = no avatar) |
| `Read()` | Avatar bytes (nil if no avatar) |
| `Write(data)` | Store new avatar, update hash |
| `Delete()` | Remove avatar, clear hash |

The avatar hash is announced in `PresenceMsg.avatarHash`. Remote peers compare hashes to decide whether to re-fetch.

### Cache (`internal/avatar/cache.go`)

Caches remote peer avatars on disk:

- Directory: `<peerDir>/cache/avatars/`
- Files: `{peerID}.png` (avatar) + `{peerID}.hash` (stored hash for validation)
- Thread-safe (sync.RWMutex)

| Method | Purpose |
| -- | -- |
| `Get(peerID, hash)` | Return cached avatar if hash matches |
| `GetAny(peerID)` | Return cached avatar ignoring hash |
| `Put(peerID, hash, data)` | Store avatar + hash |

### P2P protocol

Avatar fetching uses `/goop/avatar/1.0.0` stream protocol. Peer sends request, host responds with PNG bytes.

## Content store

### Store (`internal/content/store.go`)

Manages the peer's editable site content:

- Root: `<peerDir>/site` (configurable via `paths.site_root`)
- Thread-safe file operations with path validation

### FileInfo

```go
type FileInfo struct {
    Path  string // root-relative, forward slashes
    Size  int64
    ETag  string // sha256:<hex>
    Mod   int64  // unix seconds
    IsDir bool
}
```

### Security

- Path traversal prevention: all paths resolved against root, `ErrOutsideRoot` if escaped
- Image files (`.png`, `.jpg`, `.gif`, `.webp`, `.svg`, `.ico`, `.bmp`) must be in the `images/` folder (`ErrImagePath`)
- Forbidden paths return `ErrForbidden`

### P2P protocol

Site content is served via `/goop/site/1.0.0` stream protocol. Remote peers can browse and fetch files from the site directory.
