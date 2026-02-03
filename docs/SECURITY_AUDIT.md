# Security & Production-Readiness Audit

Systematic review of 19 concerns raised against the codebase, verified against actual code.
Each point is rated as **REAL ISSUE**, **PARTIAL**, **NON-ISSUE**, or **N/A**.

---

## Rendezvous Server

### 1. Binary / service naming mismatch

**REAL ISSUE (low severity)**

The binary is `goop2` and the rendezvous mode is invoked as `goop2 rendezvous <dir>`.
However, the systemd service file (`docs/goop-rendezvous.service`) uses old flag syntax
(`-rendezvous -addr 127.0.0.1:8787`) that no longer matches the CLI. The service file
needs updating to the current subcommand syntax.

**Action:** Update `docs/goop-rendezvous.service` ExecStart to `goop2 rendezvous /path/to/dir`.

---

### 2. Config precedence undefined

**NON-ISSUE**

Config loading is straightforward and unambiguous:
`Default()` produces hardcoded defaults, then `goop.json` is unmarshalled on top.
There are no environment variables and no CLI flags that override config values.
Precedence is: `goop.json > hardcoded defaults`. No ambiguity exists.

---

### 3. Multi-instance correctness (round-robin)

**REAL ISSUE (high severity)**

All peer state lives in an in-memory `map[string]peerRow` on each rendezvous instance.
There is no shared backend, no replication, no stickiness mechanism.

If multiple instances run behind a round-robin load balancer:
- Peer A publishes to instance 1, Peer B to instance 2
- Neither instance knows about the other's peers
- SSE subscribers on instance 1 never see Peer B's presence

The deployment docs show a multi-instance Caddy config without mentioning this limitation.

**Action:** Either:
- Document that multi-instance requires sticky sessions (e.g. `lb_policy ip_hash`), or
- Implement a shared backend (Redis, SQLite WAL on shared volume), or
- Remove the multi-instance example from docs until supported.

---

### 4. SSE durability

**PARTIAL (medium severity)**

Current mitigations:
- Buffered channels (64 slots) per SSE client
- Slow clients get messages dropped (non-blocking send), preventing server stalls
- 25-second keepalive pings
- systemd `LimitNOFILE=65536`
- Client reconnects with exponential backoff (up to 5s)

Gaps:
- No maximum connection count; unbounded SSE clients can exhaust file descriptors
- No per-IP connection limit on `/events`
- No idle timeout on long-lived connections (by design for SSE, but attackers can hold slots)

**Action:** Add a configurable max-connections guard. Rely on reverse proxy rate limiting
for per-IP limits (already documented in Caddy example).

---

### 5. Publish endpoint abuse

**REAL ISSUE (medium severity)**

`/publish` has input validation (max 256 char PeerID, max 4096 char Content,
type whitelist), but:
- No per-IP rate limiting (server-side)
- No body size limit on the POST (json.Decoder reads unbounded)
- No per-peer publish frequency cap
- Broadcast happens for every valid message; spam floods all SSE clients

The Caddy example includes `rate_limit {remote.ip} 100r/m` which helps, but the server
itself is unprotected when run without a proxy.

**Action:** Add server-side rate limiting on `/publish` (per-IP, token bucket).
Add `http.MaxBytesReader` on the request body.

---

### 6. Health semantics

**REAL ISSUE (low severity)**

`/healthz` unconditionally returns `200 "ok"`. It does not verify:
- Whether the server can accept new connections
- Peer count or goroutine health
- Whether the cleanup goroutine is running

For single-instance deployments this is fine. For multi-instance behind a load balancer,
a broken instance stays in rotation indefinitely.

**Action:** Return a JSON status with peer count and uptime. Mark unhealthy if
internal goroutines have panicked.

---

## Lua Engine

### 7. Rate limits too coarse

**REAL ISSUE (medium severity)**

Chat commands and data functions share a single `rateLimiter` instance with one global
counter and one per-peer-per-function counter. A peer making many data function calls
can exhaust the global budget and block other peers' chat commands.

Per-function `@rate_limit` annotations exist but only control per-peer limits,
not global partitioning.

No burst handling (no token bucket); the limiter uses a sliding-window timestamp array.

**Action:** Split into separate global budgets for chat vs data, or at minimum
use independent global counters. Consider token-bucket for burst tolerance.

---

### 8. SSRF edge cases

**PARTIAL (low severity)**

`checkSSRF()` in `api.go` resolves hostnames via `net.LookupIP()` and blocks
`IsLoopback()`, `IsPrivate()`, `IsLinkLocalUnicast()`, `IsLinkLocalMulticast()`.
This covers IPv4 and IPv6 private ranges correctly.

Gaps:
- **DNS rebinding (TOCTOU):** IP is checked at lookup time, but the HTTP client
  could resolve to a different IP. Mitigated somewhat by the short 10s timeout,
  but not fully prevented.
- **Port validation:** No check on target port; internal services on non-standard
  ports are reachable.
- **Mixed resolution:** If DNS returns both public and private IPs, only one
  needs to be private to block. Current code checks all resolved IPs, so this
  is actually handled correctly.

**Action:** Pin the resolved IP in the HTTP transport dialer to prevent TOCTOU.
Consider blocking well-known internal ports (e.g., 6379, 5432, 3306).

---

### 9. Memory/timeout enforcement must be hard

**PARTIAL (medium severity)**

Timeout: Uses `context.WithTimeout` + goroutine + `L.Close()` on deadline.
This is a hard kill (VM state is destroyed), not cooperative polling.
Effective but has a race: the goroutine may still run briefly after `L.Close()`.

Memory: **No VM heap limit.** KV store capped at 64KB, query results at 1MB,
HTTP responses at 1MB. But the Lua VM itself can allocate unbounded tables/strings.
A malicious script can OOM the process.

**Action:** Set a memory limit on the Lua VM. The gopher-lua library supports
`SetMx` for instruction count limits; use that as a proxy for memory. Alternatively,
run Lua in a subprocess with cgroup memory limits.

---

### 10. Clear separation of read vs write

**PARTIAL (medium severity)**

Data functions get both `db.query` (read) and `db.exec` (write) APIs.
SQL validation whitelists: query allows only SELECT/WITH, exec allows only
INSERT/UPDATE/DELETE/REPLACE. Multi-statement injection is blocked.

However, `db.exec` has no row-level or table-level ownership enforcement.
A data function can INSERT/UPDATE/DELETE any row in any table, not just
rows belonging to its owner.

Chat scripts have **no** database access at all (correct).

**Action:** If data functions should be read-only, remove `db.exec` from the
data VM sandbox. If writes are intentional, document the trust model and
consider per-table write permissions via manifest config.

---

## Groups & Distributed Compute

### 11. Atomic work claiming

**N/A**

No distributed work queue or `claim_work` mechanism exists in the codebase.
The "distributed compute" doc describes a future design, not current code.

---

### 12. TTL re-queuing

**N/A**

No task queue exists. The only TTL is for peer presence (20s default),
which correctly prunes stale peers from the in-memory table.

---

### 13. Partial fan-out handling

**REAL ISSUE (medium severity)**

Group broadcasts (`hostedGroup.broadcast()`) iterate members synchronously.
Each `encoder.Encode(msg)` write is unbuffered; a slow peer blocks the
entire broadcast loop. There are no per-peer timeouts or partial-result
aggregation.

For group operations that expect responses from multiple peers, a single
slow/dead peer can stall the coordinator.

**Action:** Add per-peer write timeouts (e.g., 5s deadline per send).
Use goroutines for fan-out with `sync.WaitGroup` and per-peer context.

---

### 14. Worker opt-out path

**NON-ISSUE**

`LeaveGroup()` sends a leave message, closes the stream, removes the local
subscription, and notifies listeners. The host side detects stream closure,
removes the member from its map, and broadcasts an updated member list.

Ungraceful disconnects (crashes) are detected via stream closure and handled
the same way. Functional and clean.

---

### 15. Result trust model

**PARTIAL (low severity for current use)**

Peer identity is cryptographically verified via libp2p Ed25519 keys.
The host injects `msg.From = mc.peerID` server-side, preventing spoofing
within groups.

However, result data from remote data operations has no integrity checking
(no signatures, no checksums). A compromised peer can return arbitrary data
from `RemoteDataOp`.

For the current use case (collaborative sites), this is acceptable.
For sensitive distributed compute, it would need cross-validation.

**Action:** Document the trust model. For sensitive operations, add HMAC
signatures on result payloads.

---

## System Boundaries / Attack Surface

### 16. Do not expose rendezvous directly

**NON-ISSUE (already documented)**

The deployment docs explicitly state: "Always use reverse proxy with HTTPS"
and "Don't expose the rendezvous server directly." The systemd service binds
to `127.0.0.1`. Caddy and nginx examples are provided.

The server has no built-in TLS, which is the standard Go pattern (terminate
TLS at the proxy). This is fine.

---

### 17. No script exposure via site serving

**PARTIAL (medium severity)**

`lua/` directory: Correctly blocked. P2P site handler explicitly rejects
paths starting with `lua/` with `"ERR forbidden"`.

`.state/` directory: **Not blocked.** KV state files (JSON) stored in
`.state/` could be readable via `/p/{peerID}/.state/filename.json`.
This leaks internal Lua script state to remote peers.

**Action:** Add `.state/` to the forbidden path list alongside `lua/`.

---

### 18. Backpressure on `/events`

**PARTIAL (see point 4)**

Same as SSE durability analysis. Slow clients get messages dropped (good),
but no connection count limit (bad). Covered in point 4.

---

### 19. Clear failure modes

**PARTIAL (medium severity)**

Defined:
- Peer restart: auto-rejoin via subscription DB + `reconnectSubscriptions()`
- Hub restart: groups reload from DB
- Network partition: presence stops, rendezvous reconnect with backoff
- P2P stream timeout: per-operation context deadlines (30s data ops, 8s group rejoin)

Undefined:
- Group host disappears mid-session: no leader election, no failover.
  Members detect via stream closure but the group is effectively dead.
- Slow peer in broadcast: blocks all others (see point 13)
- No distributed consensus for group state
- No WAL or activity log for group state recovery

**Action:** Document failure modes explicitly. For host failure, consider
allowing members to elect a new host or at minimum surface the failure clearly
in the UI.

---

## Priority Summary

| # | Issue | Severity | Effort |
|---|-------|----------|--------|
| 3 | Multi-instance peers disappear | High | High |
| 9 | No Lua VM memory limit | High | Medium |
| 5 | Publish endpoint abuse (no rate limit) | Medium | Low |
| 7 | Shared rate limiter (chat + data) | Medium | Low |
| 13 | Broadcast blocks on slow peer | Medium | Medium |
| 17 | `.state/` directory exposed | Medium | Low |
| 10 | Data functions can write any row | Medium | Low |
| 4 | No SSE connection limit | Medium | Low |
| 19 | Undefined host-failure mode | Medium | Medium |
| 1 | Systemd service file outdated | Low | Trivial |
| 6 | Health endpoint too simple | Low | Low |
| 8 | SSRF DNS rebinding TOCTOU | Low | Medium |
| 15 | No result data integrity | Low | Medium |
| 2 | Config precedence | Non-issue | -- |
| 14 | Worker opt-out | Non-issue | -- |
| 16 | Rendezvous exposure | Non-issue | -- |
| 18 | SSE backpressure | Duplicate of 4 | -- |
| 11 | Atomic work claiming | N/A (not built) | -- |
| 12 | TTL re-queuing | N/A (not built) | -- |
