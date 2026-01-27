# Rendezvous Server Deployment Guide

This guide covers deploying the standalone Goop² Rendezvous Server for production use.

## Overview

The rendezvous server provides:
- **Peer Discovery**: Helps peers find each other via libp2p rendezvous protocol
- **Web Monitoring UI**: Real-time view of connected peers via web interface
- **SSE Events**: Server-Sent Events stream for live peer updates
- **REST API**: JSON endpoints for programmatic access

## Quick Start

### 1. Build the Binary

```bash
cd /home/peter/Projects/goop2
go build -o goop2
```

For optimized production build:
```bash
go build -ldflags="-s -w" -o goop2
```

### 2. Test Locally

```bash
./goop2 -rendezvous -addr localhost:8787
```

Visit http://localhost:8787 to see the monitoring UI.

### 3. Production Deployment

#### Create User and Directories

```bash
sudo useradd -r -s /bin/false goop
sudo mkdir -p /opt/goop /var/log/goop
sudo cp goop2 /opt/goop/
sudo chown -R goop:goop /opt/goop /var/log/goop
sudo chmod 755 /opt/goop/goop2
```

#### Install Systemd Service

```bash
sudo cp docs/goop-rendezvous.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable goop-rendezvous
sudo systemctl start goop-rendezvous
```

#### Check Status

```bash
sudo systemctl status goop-rendezvous
sudo journalctl -u goop-rendezvous -f
```

## Reverse Proxy Setup

### Caddy Configuration

See [Caddyfile.example](./Caddyfile.example) for detailed configurations.

**Simple subdomain setup:**

```caddyfile
rendezvous.yourdomain.com {
    reverse_proxy localhost:8787
}
```

**With path-based routing:**

```caddyfile
yourdomain.com {
    handle /rendezvous* {
        reverse_proxy localhost:8787
    }
}
```

Install and reload Caddy:
```bash
sudo systemctl reload caddy
```

### Nginx Alternative

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

## Endpoints

### Web UI
- `GET /` - Real-time peer monitoring dashboard
  - Shows connected peers with IDs, addresses, and timestamps
  - Auto-refreshes via Server-Sent Events

### REST API
- `GET /peers.json` - JSON list of all registered peers
- `POST /publish` - Register a peer (used by clients)
- `GET /events` - SSE stream of peer events (joins/leaves)
- `GET /healthz` - Health check endpoint (returns "ok")

### Example API Usage

**Check connected peers:**
```bash
curl https://rendezvous.yourdomain.com/peers.json
```

**Health check:**
```bash
curl https://rendezvous.yourdomain.com/healthz
```

**Monitor events (SSE):**
```bash
curl -N https://rendezvous.yourdomain.com/events
```

## Configuration

### Command-Line Flags

```bash
./rendezvous -h
```

Options:
- `-addr` - Listen address (default: localhost:8787)
  - Use `0.0.0.0:8787` to listen on all interfaces
  - Use `127.0.0.1:8787` for localhost only (recommended with reverse proxy)

### Environment Variables

Set in systemd service file:
```ini
[Service]
Environment="GOOP_RENDEZVOUS_ADDR=127.0.0.1:8787"
Environment="GOOP_LOG_LEVEL=info"
```

## Firewall Configuration

If exposing directly (not recommended, use reverse proxy instead):

```bash
# UFW (Ubuntu)
sudo ufw allow 8787/tcp

# Firewalld (CentOS/RHEL)
sudo firewall-cmd --permanent --add-port=8787/tcp
sudo firewall-cmd --reload
```

**Best practice:** Only expose via reverse proxy on ports 80/443.

## Monitoring

### Systemd Status
```bash
sudo systemctl status goop-rendezvous
```

### Logs
```bash
# Follow logs
sudo journalctl -u goop-rendezvous -f

# Last 100 lines
sudo journalctl -u goop-rendezvous -n 100

# Today's logs
sudo journalctl -u goop-rendezvous --since today
```

### Web Interface
Visit your configured domain to see:
- Currently connected peers
- Peer IDs and multiaddresses
- Connection timestamps
- Real-time join/leave events

### Metrics

The server logs include:
- Peer registration events
- Connection attempts
- HTTP request logs
- Error conditions

## Load Balancing

For high-availability deployments, run multiple instances:

**Caddyfile:**
```caddyfile
rendezvous.yourdomain.com {
    reverse_proxy localhost:8787 localhost:8788 localhost:8789 {
        lb_policy round_robin
        health_uri /healthz
        health_interval 10s
    }
}
```

Start multiple instances:
```bash
./rendezvous -addr localhost:8787 &
./rendezvous -addr localhost:8788 &
./rendezvous -addr localhost:8789 &
```

## Security Considerations

1. **Always use reverse proxy with HTTPS** - Don't expose the rendezvous server directly
2. **Bind to localhost** - Use `-addr 127.0.0.1:8787` when behind reverse proxy
3. **Rate limiting** - Configure in Caddy/Nginx to prevent abuse
4. **Authentication** - See Caddyfile.example for basic auth setup on sensitive endpoints
5. **Firewall** - Only allow connections from your reverse proxy
6. **Updates** - Keep the binary updated for security patches

## Troubleshooting

### Service won't start
```bash
# Check logs for errors
sudo journalctl -u goop-rendezvous -n 50

# Verify binary permissions
ls -la /opt/goop/rendezvous

# Test manually
sudo -u goop /opt/goop/rendezvous -addr localhost:8787
```

### Can't connect through reverse proxy
```bash
# Test direct connection
curl http://localhost:8787/healthz

# Check Caddy/Nginx logs
sudo journalctl -u caddy -f
sudo tail -f /var/log/nginx/error.log

# Verify proxy is running
sudo systemctl status caddy
```

### Peers not registering
- Check firewall rules
- Verify peer configuration points to correct rendezvous URL
- Check server logs for connection attempts
- Ensure reverse proxy isn't blocking POST requests

### SSE events not working
- Verify proxy doesn't buffer responses (proxy_buffering off)
- Check proxy timeout settings
- Test SSE directly: `curl -N http://localhost:8787/events`

## Client Configuration

Configure Goop² clients to use your rendezvous server:

**goop.json:**
```json
{
  "site": "My Site",
  "peerName": "my-peer",
  "rendezvous": "https://rendezvous.yourdomain.com"
}
```

Clients will automatically register with the server and appear in the web UI.

## Backup and Recovery

The rendezvous server is stateless - all peer data is in-memory. No backup needed.

To migrate:
1. Stop old server
2. Update DNS or reverse proxy to point to new server
3. Start new server
4. Peers will re-register automatically

## Performance

Expected capacity:
- **1000+ peers** on a single instance (2 vCPU, 1GB RAM)
- **< 1ms** response time for API requests
- **< 10MB** memory per 1000 peers
- **SSE connections** count toward open file descriptors (increase limits if needed)

## Updates

To update the server:

```bash
# Build new binary
cd /home/peter/Projects/goop2
go build -ldflags="-s -w" -o goop2

# Stop service
sudo systemctl stop goop-rendezvous

# Replace binary
sudo cp goop2 /opt/goop/
sudo chown goop:goop /opt/goop/goop2
sudo chmod 755 /opt/goop/goop2

# Start service
sudo systemctl start goop-rendezvous
```

Zero-downtime updates with multiple instances:
1. Update instance 1, wait for health check
2. Update instance 2, wait for health check
3. Update instance 3, etc.

## Support

For issues or questions:
- Check logs: `sudo journalctl -u goop-rendezvous -f`
- Test health: `curl https://your-domain/healthz`
- Verify config: Review Caddyfile and service file
- Check firewall: Ensure ports are open and accessible
