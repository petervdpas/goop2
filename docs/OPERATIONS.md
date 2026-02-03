# Goop² Operations Guide

## Building

```bash
# Standard build
go build -o goop2

# Optimized production build (smaller binary)
go build -ldflags="-s -w" -trimpath -o goop2

# Static binary (Alpine/musl)
CGO_ENABLED=0 go build -ldflags="-s -w" -o goop2

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o goop2-linux-amd64
GOOS=linux GOARCH=arm64 go build -o goop2-linux-arm64
```

---

## CLI Modes

Goop² is a single executable that runs in three modes.

### 1. Desktop Mode (Default)

Launch the GUI for managing multiple peers:

```bash
./goop2
```

Features: peer creation/deletion, start/stop from UI, theme synchronization, embedded assets.

### 2. Peer Mode

Run a full peer node from the command line without the desktop UI:

```bash
goop2 peer <peer-directory>
```

**Example:**
```bash
./goop2 peer ./peers/mysite
./goop2 peer /home/user/peers/blog
```

**What it does:**
- Loads `goop.json` from the peer directory
- Opens or creates `data.db` SQLite database
- Serves static site from `site/` subdirectory
- Joins P2P network (mDNS + libp2p)
- Accepts remote data operations via `/goop/data/1.0.0`
- Starts local viewer HTTP server (if configured)
- Optionally hosts rendezvous server (if configured)

**Peer directory structure:**
```
peers/mysite/
├── goop.json          # Configuration
├── data.db            # SQLite database (auto-created)
└── site/              # Your static site
    ├── index.html
    └── assets/
```

### 3. Rendezvous Mode

Run a standalone rendezvous server (no P2P node):

```bash
goop2 rendezvous <peer-directory>
```

The peer's `goop.json` should have `rendezvous_host: true` configured. This starts the rendezvous HTTP server for peer discovery plus a **minimal settings viewer** with Settings (`/self`) and Logs (`/logs`) pages.

### CLI vs Desktop Comparison

| Feature | CLI Peer Mode | Desktop Mode |
|---------|----------|-------------|
| **Command** | `goop2 peer <dir>` | `goop2` |
| **Memory** | ~30-50 MB | ~80-150 MB |
| **GUI** | None | Full UI |
| **Multi-peer** | Via multiple processes | Built-in manager |
| **Automation** | Excellent | Limited |
| **Server deployment** | Ideal | Not recommended |

---

## Configuration

All configuration is via `goop.json` in the peer directory. There are no environment variables or CLI flags for configuration.

### Peer Config Example

```json
{
  "profile": {
    "label": "My Production Site"
  },
  "p2p": {
    "listen_port": 4001,
    "mdns_tag": "goop-prod"
  },
  "presence": {
    "ttl_seconds": 120,
    "heartbeat_seconds": 30,
    "rendezvous_wan": "https://rendezvous.yourdomain.com"
  },
  "viewer": {
    "http_addr": "127.0.0.1:8080"
  }
}
```

**Key settings:**
- `viewer.http_addr` — Bind to localhost if behind reverse proxy
- `p2p.listen_port` — Unique per peer on same host
- `presence.rendezvous_wan` — Your production rendezvous server
- `presence.rendezvous_host` — Only if this peer hosts rendezvous

### Rendezvous Config Example

```json
{
  "profile": {
    "label": "My Rendezvous Server",
    "email": "admin@example.com"
  },
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787
  },
  "viewer": {
    "http_addr": "127.0.0.1:9090"
  }
}
```

Key rendezvous settings in `goop.json` under `"presence"`:

| Setting | Default | Description |
|---------|---------|-------------|
| `rendezvous_host` | `false` | Enable the rendezvous server |
| `rendezvous_port` | `8787` | Listen port (binds to `127.0.0.1`) |
| `rendezvous_only` | `false` | Run only rendezvous (no P2P node) |
| `templates_dir` | `"templates"` | Directory for store templates (relative to peer dir) |
| `peer_db_path` | `""` | SQLite path for peer state persistence (see Load Balancing) |

---

## Deployment Patterns

### Pattern 1: Single Peer Server

**Systemd service:**
```ini
[Unit]
Description=Goop² Peer - My Blog
After=network.target

[Service]
Type=simple
User=goop
Group=goop
WorkingDirectory=/opt/goop/peers/blog
ExecStart=/opt/goop/goop2 peer /opt/goop/peers/blog
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**Deploy:**
```bash
sudo useradd -r -s /bin/false goop
sudo mkdir -p /opt/goop/peers/blog/site
sudo cp goop2 /opt/goop/
sudo cp peers/blog/goop.json /opt/goop/peers/blog/
sudo cp -r peers/blog/site/* /opt/goop/peers/blog/site/
sudo chown -R goop:goop /opt/goop
sudo cp goop-peer.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goop-peer
```

### Pattern 2: Multiple Peers on One Host

Run multiple peer services with different ports. Ensure each peer's `goop.json` uses different P2P listen ports, viewer HTTP addresses, and rendezvous ports.

### Pattern 3: Rendezvous + Multiple Peers

```bash
# Start rendezvous server
./goop2 rendezvous ./peers/server

# Configure peers to use it (in each peer's goop.json):
# "presence": { "rendezvous_wan": "http://127.0.0.1:8787" }

# Start peers
./goop2 peer ./peers/site1
./goop2 peer ./peers/site2
```

### Pattern 4: Container Deployment

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -ldflags="-s -w" -o goop2 .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /opt/goop
COPY --from=builder /app/goop2 .
COPY peers/mysite/ ./peers/mysite/

EXPOSE 8080 4001
CMD ["./goop2", "peer", "./peers/mysite"]
```

---

## Rendezvous Server Deployment

### Quick Start

Create a peer directory with a `goop.json` that enables the rendezvous server:

```bash
mkdir -p /tmp/rv-peer
cat > /tmp/rv-peer/goop.json <<'EOF'
{
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787
  }
}
EOF
./goop2 rendezvous /tmp/rv-peer
```

Visit http://localhost:8787 to see the monitoring UI.

### Rendezvous-Only Mode

When configured with `rendezvous_only: true`, only the rendezvous server runs — no libp2p P2P node. A **minimal settings viewer** starts alongside, providing:
- **Settings page** (`/self`) — edit peer label, email, and config
- **Logs page** (`/logs`) — live log monitoring with SSE tail
- **Rendezvous dashboard link** — quick access to the rendezvous web UI

### Desktop Launcher Integration

The desktop launcher (Wails) automatically detects rendezvous-only peers:
- Rendezvous peers display a **"Rendezvous"** badge in the peer list
- The Start button changes to **"Configure"** for rendezvous peers
- Starting a rendezvous peer opens Settings (`/self`) instead of the peer list

### Production Deployment

**Create user and directories:**
```bash
sudo useradd -r -s /bin/false goop
sudo mkdir -p /opt/goop /var/log/goop
sudo cp goop2 /opt/goop/
sudo chown -R goop:goop /opt/goop /var/log/goop
sudo chmod 755 /opt/goop/goop2
```

**Install systemd service:**
```bash
sudo cp docs/goop-rendezvous.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable goop-rendezvous
sudo systemctl start goop-rendezvous
```

**Check status:**
```bash
sudo systemctl status goop-rendezvous
sudo journalctl -u goop-rendezvous -f
```

### Rendezvous Endpoints

**Web UI:**
- `GET /` — Real-time peer monitoring dashboard (auto-refreshes via SSE)

**REST API:**
- `GET /peers.json` — JSON list of all registered peers
- `GET /logs.json` — JSON-formatted server log entries
- `POST /publish` — Register a peer (used by clients)
- `GET /events` — SSE stream of peer events (joins/leaves)
- `GET /healthz` — Health check endpoint (returns "ok")

**Template Store API:**
- `GET /api/templates` — List available templates
- `GET /api/templates/<name>/manifest.json` — Template metadata
- `GET /api/templates/<name>/bundle` — Download template as tar.gz

**Example usage:**
```bash
curl https://rendezvous.yourdomain.com/peers.json
curl https://rendezvous.yourdomain.com/healthz
curl -N https://rendezvous.yourdomain.com/events
```

### Load Balancing

For high-availability deployments, run multiple instances.

> **Important:** By default, peer state is in-memory per instance. Without a shared
> peer database, each instance only sees the peers that published to it. Use one of
> these strategies:
>
> 1. **Sticky sessions (simplest):** Use `ip_hash` so each client always hits the
>    same backend. SSE works correctly and peers don't "disappear."
> 2. **Shared peer DB:** Set `peer_db_path` in each instance's `goop.json` to the
>    same SQLite file on a shared volume. Instances sync peer state every 3 seconds.
>    With a shared DB, `round_robin` is acceptable.

**Caddyfile (sticky sessions):**
```caddyfile
rendezvous.yourdomain.com {
    reverse_proxy localhost:8787 localhost:8788 localhost:8789 {
        lb_policy ip_hash
        health_uri /healthz
        health_interval 10s
    }
}
```

**Shared peer DB example `goop.json`:**
```json
{
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787,
    "peer_db_path": "/shared/volume/peers.db"
  }
}
```

Start multiple instances (each with its own peer directory but sharing the DB file):
```bash
./goop2 rendezvous /opt/goop/peer1 &
./goop2 rendezvous /opt/goop/peer2 &
./goop2 rendezvous /opt/goop/peer3 &
```

### Client Configuration

Configure Goop² clients to use your rendezvous server:

```json
{
  "site": "My Site",
  "peerName": "my-peer",
  "rendezvous": "https://rendezvous.yourdomain.com"
}
```

### Backup and Recovery

By default, the rendezvous server is stateless — all peer data is in-memory and peers
re-register automatically after a restart.

If `peer_db_path` is configured, peer state is persisted in a SQLite file. This file
can be backed up, but it is not critical — peers will re-register within seconds of
a restart even without it.

### Updates

```bash
# Build new binary
go build -ldflags="-s -w" -o goop2

# Stop, replace, start
sudo systemctl stop goop-rendezvous
sudo cp goop2 /opt/goop/
sudo chown goop:goop /opt/goop/goop2
sudo chmod 755 /opt/goop/goop2
sudo systemctl start goop-rendezvous
```

Zero-downtime updates with multiple instances (requires shared `peer_db_path` or
sticky sessions so peers survive the rolling restart):
1. Update instance 1, wait for health check
2. Update instance 2, wait for health check
3. Update instance 3, etc.

### Performance

Expected capacity:
- **1000+ peers** on a single instance (2 vCPU, 1GB RAM)
- **< 1ms** response time for API requests
- **< 10MB** memory per 1000 peers
- **SSE connections** count toward open file descriptors (increase limits if needed)

---

## Reverse Proxy Setup

### Caddy

See [Caddyfile.example](./Caddyfile.example) for detailed configurations.

**Simple subdomain:**
```caddyfile
rendezvous.yourdomain.com {
    reverse_proxy localhost:8787
}
```

**Path-based routing:**
```caddyfile
yourdomain.com {
    handle /rendezvous* {
        reverse_proxy localhost:8787
    }
}
```

**Peer viewer:**
```caddyfile
mysite.yourdomain.com {
    reverse_proxy localhost:8080
}
```

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name rendezvous.yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/rendezvous.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/rendezvous.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;

        # Important for SSE streams
        proxy_buffering off;
        proxy_read_timeout 86400;
    }
}
```

---

## Firewall Configuration

If exposing directly (not recommended — use reverse proxy instead):

```bash
# UFW (Ubuntu)
sudo ufw allow 8787/tcp

# Firewalld (CentOS/RHEL)
sudo firewall-cmd --permanent --add-port=8787/tcp
sudo firewall-cmd --reload
```

**Best practice:** Only expose via reverse proxy on ports 80/443.

---

## Monitoring

### Systemd

```bash
sudo systemctl status goop-rendezvous
sudo journalctl -u goop-rendezvous -f
sudo journalctl -u goop-rendezvous -n 100
sudo journalctl -u goop-rendezvous --since today
```

### Docker

```bash
docker logs -f <container-id>
```

### Viewer API (if enabled)

```bash
curl http://localhost:8080/peers
curl http://localhost:8080/api/logs
```

### Web Interface

The rendezvous monitoring dashboard shows currently connected peers, peer IDs and multiaddresses, connection timestamps, and real-time join/leave events.

---

## Security

### General Practices

1. **Run as dedicated user** — Never run as root
2. **Bind viewer to localhost** — Use reverse proxy for external access
3. **Always use reverse proxy with HTTPS** — Don't expose the rendezvous server directly
4. **Firewall rules** — Only expose necessary ports
5. **Limit file access** — Use systemd protections (ProtectHome, etc.)
6. **Rate limiting** — Configure in Caddy/Nginx to prevent abuse
7. **Updates** — Keep binaries up to date

### Security Audit Summary

Systematic review of 19 production-readiness concerns, verified against actual code.

#### Addressed (6/19)

| # | Issue | Status |
|---|-------|--------|
| 1 | Systemd service file outdated | **Fixed** — updated to current subcommand syntax |
| 2 | Config precedence undefined | **Non-issue** — `goop.json` is the only source; no env vars or CLI config flags |
| 3 | Multi-instance peers disappear | **Fixed** — SQLite WAL-mode peer DB via `peer_db_path`, 3s sync interval |
| 7 | Rate limits too coarse | **Addressed** — Per-function `@rate_limit N` annotations; keyed per peer+function |
| 9 | No Lua VM memory limit | **Fixed** — Registry size caps + process-level memory monitor (100ms poll, hard kill via `L.Close()`) |
| 14 | Worker opt-out path | **Addressed** — `LeaveGroup()` fully implemented with cleanup, notification, subscription removal |
| 16 | Rendezvous exposure | **Non-issue** — Hardcoded to `127.0.0.1`; not configurable; requires reverse proxy |

#### Partially Addressed (6/19)

| # | Issue | Notes |
|---|-------|-------|
| 4 | No SSE connection limit | 64-element buffered channels + 25s keepalive + non-blocking send exist; **no max connection limit** — unbounded clients can connect |
| 8 | SSRF DNS rebinding | `checkSSRF()` blocks loopback/private/link-local for IPv4+IPv6; **vulnerable to DNS rebinding** (TOCTOU between check and HTTP request) |
| 10 | Data functions can write any row | Chat scripts have no DB access (correct); data functions have full read+write via `goop.db.exec`; **no per-table permissions** — intentional design (site owner deploys scripts) |
| 13 | Broadcast blocks on slow peer | Rendezvous SSE: non-blocking send (safe). **Group broadcast: blocking** `Encode()` under lock — slow peer blocks all others |
| 17 | Script/state exposure | P2P protocol blocks `lua/` directory (correct). **HTTP viewer self-serve does not block `lua/`** — Lua source readable via local viewer |
| 19 | Undefined host-failure mode | Hub restart: peers re-register within 5s (good). **Group host crash: detection depends on TCP timeout (2-9 min)**; no application-level group heartbeat; auto-reconnect is startup-only |

#### Not Addressed (5/19)

| # | Issue | Notes |
|---|-------|-------|
| 5 | Publish endpoint abuse | **No server-side rate limiting** on `/publish`; any client can flood; relies entirely on reverse proxy |
| 6 | Health endpoint too simple | `/healthz` returns `"ok"` unconditionally; does not check DB connectivity, memory, or goroutine count |
| 15 | No result data integrity | No message signatures or cross-validation; identity enforced by host overwriting `From` field |
| 18 | Backpressure on `/events` | **No max SSE connection limit**; no per-IP limit; no idle eviction; limited only by OS file descriptors |

#### Not Applicable (2/19)

- **11. Atomic work claiming** — No work queue implemented (future design)
- **12. TTL re-queuing** — No task queue implemented (future design)

---

## Troubleshooting

### Peer won't start

```bash
./goop2 peer ./peers/mysite
ls -la ./peers/mysite/site/
netstat -tlnp | grep <port>
```

### Rendezvous service won't start

```bash
sudo journalctl -u goop-rendezvous -n 50
ls -la /opt/goop/goop2
sudo -u goop /opt/goop/goop2 rendezvous /opt/goop/peer
```

### Can't connect through reverse proxy

```bash
curl http://localhost:8787/healthz
sudo journalctl -u caddy -f
sudo tail -f /var/log/nginx/error.log
sudo systemctl status caddy
```

### Can't connect to other peers

```bash
avahi-browse -a                                    # Verify mDNS (LAN)
sudo ufw status                                     # Check firewall
curl http://your-rendezvous-server/peers.json       # Test rendezvous
```

### Peers not registering

- Check firewall rules
- Verify peer configuration points to correct rendezvous URL
- Check server logs for connection attempts
- Ensure reverse proxy isn't blocking POST requests

### SSE events not working

- Verify proxy doesn't buffer responses (`proxy_buffering off`)
- Check proxy timeout settings
- Test SSE directly: `curl -N http://localhost:8787/events`

### High memory usage

```bash
curl http://localhost:8080/api/peers | jq length
top -p $(pgrep goop2)
```

---

## Support

- Check logs: `sudo journalctl -u goop-rendezvous -f`
- Test health: `curl https://your-domain/healthz`
- Verify config: Review Caddyfile and service file
- Check firewall: Ensure ports are open and accessible
