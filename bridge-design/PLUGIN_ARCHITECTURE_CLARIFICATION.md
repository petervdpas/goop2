# Plugin Architecture: Bridge as Just Another Service

**Date**: 2026-03-02
**Purpose**: Clarify that the HTTPS Bridge is NOT special - it follows the exact same plugin architecture as credits, email, registration, templates

---

## The Model: Static Endpoint Configuration

Each service in goop2-services is independent with:
1. Its own **binary** (`goop2-service-{name}`)
2. Its own **config.json** (declares dependencies as static URLs)
3. Its own **systemd unit**
4. HTTP communication with other services

**NO dynamic discovery, NO service registry, NO capability routing.**

---

## Current Pattern (Other Services)

### Example: Credits Service

**credits/config.json** (declares dependencies):
```json
{
  "addr": ":8800",
  "db": "data/credits.db",
  "registration_url": "http://localhost:8801",
  "templates_url": "http://localhost:8803",
  "admin_token": "secret"
}
```

**What it does**:
- Runs on `:8800`
- Calls registration service at `http://localhost:8801` when needed
- Calls templates service at `http://localhost:8803` when needed
- No service discovers it; consumers must know its URL

### Example: Registration Service

**registrations/config.json**:
```json
{
  "addr": ":8801",
  "db": "data/registrations.db",
  "credits_url": "http://localhost:8800",
  "email_url": "http://localhost:8802"
}
```

**Dependency chain**: registration → credits → templates (but all static)

---

## The Bridge Follows This EXACT Pattern

### Bridge Service (New)

**https/config.json** (declares dependencies):
```json
{
  "addr": ":8804",
  "tls_cert": "/etc/goop2-services/https-bridge/cert.pem",
  "tls_key": "/etc/goop2-services/https-bridge/key.pem",
  "relay_url": "http://localhost:8888",
  "relay_auth_token": "secret-internal-token",
  "max_virtual_peers": 10000,
  "grace_period": "5s",
  "keep_alive_timeout": "30s",
  "log_level": "info"
}
```

**What it does**:
- Runs on `:8804` (HTTPS for external clients)
- Calls relay at `http://localhost:8888` (internal HTTP API)
- No service discovers it; relay must know its URL

### Main Relay (Existing)

**goop2 config.json** (updated to include bridge):
```json
{
  "listener": {
    "addr": "0.0.0.0:5001"
  },
  "internal_api": {
    "port": 8888,
    "auth_token": "secret-internal-token"
  },
  "bridge": {
    "url": "http://localhost:8804",
    "enabled": true
  }
}
```

**What relay does**:
- Runs on `:5001` (P2P)
- Exposes `/internal/api/*` on port `:8888` for bridge to call
- Knows bridge is at `http://localhost:8804` (static config)
- No service discovers it; bridge must know relay's internal API URL

---

## Communication Pattern

```
┌─────────────────────────────────────────────────┐
│         goop2-services Plugin Architecture      │
├─────────────────────────────────────────────────┤
│                                                 │
│  Service A                   Service B          │
│  (port 8800)                 (port 8801)        │
│  config.json:                config.json:       │
│  {                           {                  │
│    "service_b_url":      →     "service_a_url": │
│    "http://localhost:8801"     "http://localhost:8800"
│  }                           }                  │
│     ↓                           ↓               │
│  HTTP calls (static URLs)       HTTP calls      │
│                                 (static URLs)   │
└─────────────────────────────────────────────────┘

For HTTPS Bridge:

┌─────────────────────────────────────────────────┐
│         HTTPS Bridge (New Service)              │
├─────────────────────────────────────────────────┤
│                                                 │
│  Bridge Service              Relay Service      │
│  (port 8804)                 (port 8888)        │
│  config.json:                config.json:       │
│  {                           {                  │
│    "relay_url":          →     "bridge_url":    │
│    "http://localhost:8888"     "http://localhost:8804"
│  }                           }                  │
│     ↓                           ↓               │
│  HTTP calls to relay            HTTP accepts    │
│  /internal/api/*                from bridge     │
│                                                 │
│  ↓ Bridge Accepts HTTPS Clients                │
│  Clients (HTTPS:443)                           │
│                                                 │
└─────────────────────────────────────────────────┘
```

---

## Deployment: Static Multi-Instance Pattern

Just like you could deploy multiple email services (for redundancy), you deploy multiple bridge instances:

### Bridge Instance #1

**systemd**: `/etc/systemd/system/goop2-https-bridge@1.service`
```ini
[Service]
ExecStart=/opt/goop2-services/https-bridge/goop2-service-https-bridge \
  -config=/etc/goop2-services/https-bridge/config-1.json
```

**config-1.json**:
```json
{
  "addr": ":8804",
  "relay_url": "http://localhost:8888"
}
```

### Bridge Instance #2

**systemd**: `/etc/systemd/system/goop2-https-bridge@2.service`
```ini
[Service]
ExecStart=/opt/goop2-services/https-bridge/goop2-service-https-bridge \
  -config=/etc/goop2-services/https-bridge/config-2.json
```

**config-2.json**:
```json
{
  "addr": ":8805",
  "relay_url": "http://localhost:8888"
}
```

### Load Balancer Routing

**NGINX config** (knows all bridge instances):
```nginx
upstream goop2_bridge {
    server localhost:8804;
    server localhost:8805;
}

server {
    listen 443 ssl;
    location / {
        proxy_pass http://goop2_bridge;
    }
}
```

**Key point**: NGINX config is static too - you edit it, reload, done. No dynamic discovery.

---

## Why This Is Better Than I Originally Designed

I was overcomplicating with:
- "Virtual peer manager" (correct)
- "Internal API" (correct)
- But I was treating it as **special** compared to other services

**Actually**: The bridge IS just another service:

| Service | Dependency | Port | Config |
|---------|-----------|------|--------|
| **Email** | SMTP config | 8802 | `email/config.json` |
| **Credits** | Registration, Templates | 8800 | `credits/config.json` |
| **Registration** | Email, Credits | 8801 | `registrations/config.json` |
| **Templates** | Disk storage | 8803 | `templates/config.json` |
| **Bridge** (NEW) | Relay | 8804 | `https/config.json` |

All the same pattern.

---

## What's NOT Dynamic

❌ Services don't register themselves
❌ Services don't auto-discover each other
❌ Services don't query a registry
❌ Services don't announce capabilities
❌ Services don't hot-swap

✅ Everything is in config files
✅ Everything is static (until you restart)
✅ Everything is simple and predictable

---

## Key Files to Update

### In goop2-services:

1. **https/config.json** (bridge declares its relay dependency)
   ```json
   {
     "addr": ":8804",
     "relay_url": "http://localhost:8888",
     ...
   }
   ```

2. **https/server.go** (same pattern as email/credits)
   - LoadConfig() - reads config.json
   - NewServer() - uses config
   - RegisterRoutes() - mounts HTTP routes

3. **https/config.go** (Config struct + LoadConfig)
   ```go
   type Config struct {
     Addr         string
     RelayURL     string  // Dependency: where to call relay
     RelayAuthToken string
     ...
   }
   ```

### In goop2 (relay):

1. **internal/config/config.go** (add bridge config)
   ```go
   type Config struct {
     Server    ServerConfig
     Bridge    BridgeConfig  // NEW
     ...
   }

   type BridgeConfig struct {
     URL       string  // "http://localhost:8804"
     Enabled   bool
     AuthToken string
   }
   ```

2. **internal/app/run.go** (pass bridge config to viewer)

3. **internal/viewer/routes/relay-api.go** (NEW - internal API endpoints)
   - POST /internal/api/relay/...
   - GET /internal/api/relay/...
   - All called by bridge over HTTP

---

## Comparison Table

| Aspect | Other Services | Bridge (This Design) |
|--------|---|---|
| **Location** | goop2-services | goop2-services |
| **Config file** | `service/config.json` | `https/config.json` |
| **Declares dependencies** | Yes (via config) | Yes (relay_url in config) |
| **Binary** | `goop2-service-{name}` | `goop2-service-https-bridge` |
| **Systemd unit** | `/etc/systemd/system/goop2-{name}.service` | `/etc/systemd/system/goop2-https-bridge.service` |
| **HTTP port** | 8800-8803 | 8804 |
| **Internal communication** | HTTP | HTTP (to relay at :8888) |
| **Multi-instance** | Possible (if stateless) | Yes (stateless) |
| **Load balancer** | Optional | NGINX (for HTTPS clients) |

---

## This Simplifies the Design

By treating bridge as **just another service**:

✅ **Simpler**: Same patterns, no special cases
✅ **Consistent**: Follows goop2-services conventions
✅ **Predictable**: Same config, same deployment, same monitoring
✅ **Scalable**: Multiple instances like any other service
✅ **Maintainable**: No custom service discovery logic

---

## Updated Mental Model

**Old (Overengineered)**:
```
Relay (with special bridge handling)
└─ Bridge (special plugin)
   └─ Virtual peers
```

**Correct (Simple)**:
```
Relay (part of goop2 app)

Bridge Service (goop2-service-https-bridge in goop2-services)
  └─ Calls relay's /internal/api/* endpoints (static config)

Other Services (credits, email, etc.)
  └─ Also call other services (static config)
```

All services are equal. Bridge is not special.

---

## Conclusion

The HTTPS Bridge is implemented as:
- **One more service** in goop2-services (binary, config, systemd)
- **Static endpoint configuration** (like all other services)
- **HTTP calls** to relay at configured URL
- **No dynamic discovery** (just like the others)
- **Multiple instances** if needed (stateless)

This is **simpler, more consistent, and more maintainable** than the original design.
