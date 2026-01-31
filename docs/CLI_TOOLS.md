# CLI Tools Deployment Guide

Goop² is a single executable that can run in three modes: desktop app (with `-ui`), CLI peer (with `-peer`), or rendezvous server (with `-rendezvous`).

## Building

```bash
# Build the executable (supports all modes)
go build -o goop2

# Or with optimizations for production
go build -ldflags="-s -w" -o goop2
```

## Available Modes

### 1. Desktop Mode

Run with `-ui` flag to launch the GUI:

```bash
./goop2 -ui
```

### 2. Peer Mode

Run a full peer node from the command line without the desktop UI.

**Usage:**
```bash
goop2 peer <peer-directory>
```

**Example:**
```bash
# Run a peer from peers/mysite directory
./goop2 peer ./peers/mysite

# With absolute path
./goop2 peer /home/user/peers/blog
```

**What it does:**
- Loads `goop.json` from the peer directory
- Opens or creates `data.db` SQLite database
- Serves static site from `site/` subdirectory
- Joins P2P network (mDNS + libp2p)
- Accepts remote data operations from visiting peers via `/goop/data/1.0.0`
- Announces presence to other peers
- Starts local viewer HTTP server (if configured)
- Optionally hosts rendezvous server (if configured)

**Example peer directory structure:**
```
peers/mysite/
├── goop.json          # Configuration
├── data.db            # SQLite database (auto-created)
└── site/              # Your static site
    ├── index.html
    └── assets/
```

**Stopping the peer:**
Press `Ctrl+C` for graceful shutdown.

---

### 3. Rendezvous Mode

Run a peer configured as a rendezvous server.

**Usage:**
```bash
goop2 rendezvous <peer-directory>
```

**Example:**
```bash
# Run peer configured as rendezvous server
./goop2 rendezvous ./peers/server
```

**Note:** The peer's `goop.json` should have `rendezvousHost: true` configured.

**What it does:**
- Starts the rendezvous HTTP server for peer discovery
- Starts a **minimal settings viewer** alongside the rendezvous server
- The minimal viewer provides Settings (`/self`) and Logs (`/logs`) pages only — no peer list, editor, or site proxy
- No libp2p P2P node is started

This means rendezvous servers can be configured through the same web UI used for regular peers, without running a full P2P node.

See [RENDEZVOUS_DEPLOYMENT.md](./RENDEZVOUS_DEPLOYMENT.md) for full production deployment guide.

---

## CLI vs Desktop Application

### Desktop Mode (Default)

**Use when:**
- You want a GUI for managing multiple peers
- You need an integrated launcher and peer manager
- You prefer visual controls

**Run:**
```bash
./goop2
```

**Features:**
- Peer creation and deletion
- Start/stop peers from UI
- Theme synchronization
- Embedded assets

---

### CLI Peer Mode

**Use when:**
- Running as a server/daemon
- Deploying to headless environments
- Scripting and automation
- Running in containers or systemd services

**Run:**
```bash
./goop2 -peer -dir /path/to/peer
```

**Advantages:**
- No GUI overhead
- Lower resource usage
- Simpler deployment
- Better for servers and automation

---

## Production Deployment Patterns

### Pattern 1: Single Peer Server

Run one peer as a persistent service.

**Systemd Service:**
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
# Create user
sudo useradd -r -s /bin/false goop

# Setup directories
sudo mkdir -p /opt/goop/peers/blog/site
sudo cp goop2 /opt/goop/
sudo cp peers/blog/goop.json /opt/goop/peers/blog/
sudo cp -r peers/blog/site/* /opt/goop/peers/blog/site/
sudo chown -R goop:goop /opt/goop

# Install and start service
sudo cp goop-peer.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goop-peer
```

---

### Pattern 2: Multiple Peers on One Host

Run multiple peer services with different ports.

**peer1.service:**
```ini
[Service]
ExecStart=/opt/goop/goop2 peer /opt/goop/peers/peer1
```

**peer2.service:**
```ini
[Service]
ExecStart=/opt/goop/goop2 peer /opt/goop/peers/peer2
```

Ensure each peer's `goop.json` uses different ports:
- P2P listen ports
- Viewer HTTP addresses
- Rendezvous ports (if hosting)

---

### Pattern 3: Rendezvous + Multiple Peers

Run a dedicated rendezvous server with multiple peers connecting to it.

```bash
# Start rendezvous server (always running)
./goop2 rendezvous ./peers/server

# Configure peers to use it
# In each peer's goop.json:
{
  "presence": {
    "rendezvousWAN": "http://127.0.0.1:8787"
  }
}

# Start peers
./goop2 peer ./peers/site1
./goop2 peer ./peers/site2
```

---

### Pattern 4: Container Deployment

**Dockerfile for peer:**
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

**Build and run:**
```bash
docker build -t goop-peer .
docker run -d -p 8080:8080 -p 4001:4001 goop-peer
```

---

## Configuration Tips

### Peer Config for CLI Usage

**Example goop.json for server deployment:**
```json
{
  "profile": {
    "label": "My Production Site"
  },
  "p2p": {
    "listenPort": 4001,
    "mdnsTag": "goop-prod"
  },
  "presence": {
    "enabled": true,
    "ttlSeconds": 120,
    "heartbeatSeconds": 30,
    "rendezvousWAN": "https://rendezvous.yourdomain.com"
  },
  "viewer": {
    "httpAddr": "127.0.0.1:8080"
  }
}
```

**Key settings:**
- `viewer.httpAddr` - Bind to localhost if behind reverse proxy
- `p2p.listenPort` - Unique per peer on same host
- `presence.rendezvousWAN` - Your production rendezvous server
- `presence.rendezvousHost` - Only if this peer hosts rendezvous

---

## Reverse Proxy Setup

### With Viewer Enabled

**Caddy:**
```caddyfile
mysite.yourdomain.com {
    reverse_proxy localhost:8080
}
```

**Nginx:**
```nginx
server {
    listen 443 ssl http2;
    server_name mysite.yourdomain.com;
    
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## Monitoring

### Check Peer Status

**Systemd:**
```bash
sudo systemctl status goop-peer
sudo journalctl -u goop-peer -f
```

**Docker:**
```bash
docker logs -f <container-id>
```

### Viewer API

If viewer is enabled, monitor via HTTP:

```bash
# Check peers
curl http://localhost:8080/peers

# Get logs
curl http://localhost:8080/api/logs
```

---

## Build Optimizations

### Production Builds

**Minimal binary size:**
```bash
go build -ldflags="-s -w" -trimpath -o goop2
```

**Static binary (Alpine/musl):**
```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o goop2
```

**Cross-compilation:**
```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o goop2-linux-amd64

# Linux ARM64 (Raspberry Pi, etc.)
GOOS=linux GOARCH=arm64 go build -o goop2-linux-arm64
```

---

## Security Considerations

1. **Run as dedicated user** - Never run as root
2. **Bind viewer to localhost** - Use reverse proxy for external access
3. **Firewall rules** - Only expose necessary ports
4. **Update regularly** - Keep binaries up to date
5. **Limit file access** - Use systemd protections (ProtectHome, etc.)

---

## Comparison: CLI vs Desktop

| Feature | CLI Peer Mode | Desktop Mode |
|---------|----------|-------------|
| **Executable** | Same (`goop2`) | Same (`goop2`) |
| **Command** | `goop2 -peer -dir ...` | `goop2` |
| **Memory** | ~30-50 MB | ~80-150 MB |
| **GUI** | None | Full UI |
| **Multi-peer** | Via multiple processes | Built-in manager |
| **Automation** | Excellent | Limited |
| **Server deployment** | Ideal | Not recommended |
| **Local development** | Good | Excellent |

---

## Troubleshooting

### Peer won't start

```bash
# Check config syntax
./goop2 peer ./peers/mysite

# Verify site directory exists
ls -la ./peers/mysite/site/

# Check port availability
netstat -tlnp | grep <port>
```

### Can't connect to other peers

```bash
# Verify mDNS is working (LAN)
avahi-browse -a

# Check firewall
sudo ufw status
sudo iptables -L

# Test rendezvous connection
curl http://your-rendezvous-server/peers.json
```

### High memory usage

```bash
# Check peer count
curl http://localhost:8080/api/peers | jq length

# Monitor resource usage
top -p $(pgrep goop2)
```

---

## Examples

### Quick Local Test

```bash
# Terminal 1: Start peer A
./goop2 peer ./peers/peerA

# Terminal 2: Start peer B
./goop2 peer ./peers/peerB

# They'll discover each other via mDNS on LAN
```

### Production Setup

```bash
# Build optimized binary
go build -ldflags="-s -w" -o /tmp/goop2

# Deploy to server
scp /tmp/goop2 user@server:/opt/goop/

# Setup systemd service (see examples above)
# Start and enable
sudo systemctl enable --now goop-peer
```

---

For more information:
- [RENDEZVOUS_DEPLOYMENT.md](./RENDEZVOUS_DEPLOYMENT.md) - Rendezvous server deployment
- [Caddyfile.example](./Caddyfile.example) - Reverse proxy configurations
- [README.md](../README.md) - Project overview
