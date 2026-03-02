# HTTPS Bridge as Microservice - Assessment & Summary

**Date**: 2026-03-02
**Status**: ✅ **VIABLE** - Ready for microservice architecture in goop2-services

---

## Executive Summary

The HTTPS Bridge can be **effectively deployed as a standalone microservice** in the `goop2-services` repository. It follows the same patterns as existing services (credits, email, registration, templates) and requires minimal changes to the main relay.

---

## Quick Comparison

### Option A: Embedded in Main Relay
```
Single goop2 process
├─ P2P networking (mDNS)
├─ Groups/Realtime protocols
└─ HTTPS Bridge (new)
    └─ Clients → HTTPS:443 (encrypted)
```

**When to use**: Development, small deployments, simplicity

### Option B: Microservice (Recommended)
```
                                        Clients
                                  HTTPS:443 encrypted
                                          ↓
goop2 Relay (port 8888)             Load Balancer (NGINX)
├─ P2P networking                        ↓
├─ Groups/Realtime            ┌──────────┴──────────┐
└─ Internal API (HTTP:8888)    │                    │
    ↑ HTTP (not HTTPS)         ↓                    ↓
    │                   Bridge #1              Bridge #2
    └─ /internal/api/*  HTTP:8804            HTTP:8805
       (internal only)   WebSocket/SSE       WebSocket/SSE
```

**Why HTTP internally?** See `./HTTPS_vs_HTTP_INTERNAL.md` - it's an internal trusted network, HTTPS adds CPU cost with zero security benefit.

**When to use**: Production, mobile/web clients, horizontal scaling

---

## Why Microservice Works

| Factor | Assessment |
|--------|-----------|
| **Stateless** | ✅ Yes - virtual peers are ephemeral, state in relay |
| **Clear API** | ✅ Yes - HTTP/JSON between bridge and relay |
| **Pattern Match** | ✅ Yes - follows goop2-services exactly |
| **Scalability** | ✅ Yes - deploy N instances behind load balancer |
| **Fault Isolation** | ✅ Yes - bridge crash doesn't affect P2P |
| **Latency Impact** | ⚠️ Low - 5-10ms over localhost HTTP is acceptable |

---

## Architecture Overview

### Communication Flow (with Protocol Clarification)

```
Mobile/Web Clients (untrusted internet)
      ↓
HTTPS:443 (TLS encrypted, authenticated)
      ↓
Load Balancer (NGINX, terminates TLS)
      ↓
HTTP:8804/8805/8806 (internal only, not encrypted)
      ↓
Bridge Instances (goop2-https-bridge)
      ↓
HTTP:8888 (internal only, not encrypted)
[See HTTPS_vs_HTTP_INTERNAL.md - internal HTTP is correct]
      ↓
Main Relay (goop2)
      ↓
P2P Network + mDNS Peers
```

**Key Insight**: Encryption terminates at load balancer. Internal services use HTTP because they're on a trusted network behind firewall.

### Key Points

1. **Virtual Peer Creation**: Bridge receives HTTPS connection → instantiates `VirtualPeer` object → synthesized peer ID (e.g., `vp-mobile-alice`)

2. **Protocol Bridge**: When virtual peer joins a group, relay treats it as a regular member. Messages flow:
   - Local peer → relay (P2P)
   - Relay → virtual peer (internal HTTP API)
   - Virtual peer → HTTPS client (WebSocket)

3. **Internal API**: Relay exposes `/internal/api/*` endpoints (port 8888, localhost-only) for bridge to:
   - Query peer/group state
   - Join groups on behalf of virtual peers
   - Receive group/realtime events
   - Publish messages

---

## Implementation Checklist (High-Level)

### Bridge Service (goop2-services)
- [ ] Create `https/` package with:
  - `config.go` - configuration loading
  - `server.go` - HTTP server + routes
  - `vpeer_manager.go` - virtual peer lifecycle
  - `gateway.go` - REST API handlers
  - `relay_client.go` - HTTP client to relay
  - `websocket.go` - WebSocket event handling
- [ ] Create `cmd/https-bridge-server/main.go`
- [ ] Add Makefile target: `make build` → `bin/goop2-service-https-bridge`
- [ ] Create `https/config.json` template
- [ ] Add systemd service file

### Main Relay Changes (goop2)
- [ ] Add `BridgeConfig` struct to relay
- [ ] Implement `/internal/api/*` endpoints
- [ ] Add routing logic for virtual peers (peerId prefix check)
- [ ] Expose internal HTTP server (port 8888)
- [ ] Secure with auth token

### Infrastructure
- [ ] TLS certificates (Let's Encrypt or CA)
- [ ] NGINX load balancer config
- [ ] Systemd units for bridge instances (3+ in production)
- [ ] Monitoring/alerting integration

---

## File Structure

```
goop2-services/
├── cmd/
│   └── https-bridge-server/
│       └── main.go                 (60 lines)
├── https/
│   ├── config.go                   (80 lines)
│   ├── config.json                 (20 lines)
│   ├── server.go                   (150 lines)
│   ├── vpeer_manager.go            (300 lines)
│   ├── gateway.go                  (400 lines)
│   ├── relay_client.go             (150 lines)
│   ├── websocket.go                (200 lines)
│   ├── events.go                   (100 lines)
│   └── server_test.go              (200 lines)
├── Makefile                        (updated)
└── README.md

goop2/ (main relay)
├── internal/
│   └── viewer/
│       └── bridge.go               (150 lines - new internal API)
└── <relay main changes>            (100 lines)
```

---

## Trade-offs Summary

| Metric | Embedded | Microservice | Winner |
|--------|----------|--------------|--------|
| Latency | <1ms | 5-10ms | Embedded |
| Scalability | Single machine | Unlimited | Microservice |
| Complexity | Low | Medium | Embedded |
| Resilience | Coupled | Isolated | Microservice |
| Operations | 1 systemd | 2-3 systemd | Embedded |
| Mobile clients | Yes | Yes | - |

**Verdict**: For development, go embedded. For production at scale, go microservice.

---

## Effort Estimate

| Component | LOC | Effort |
|-----------|-----|--------|
| Bridge service code | 800 | 3-4 days |
| Relay changes | 200 | 1 day |
| Tests | 500 | 2 days |
| Infrastructure | 100 | 1 day |
| Integration testing | - | 2 days |
| **Total** | **1,600** | **~9-10 days** |

---

## Next Steps

1. **Read the full design**: `./HTTPS_BRIDGE_DESIGN.md` (2,471 lines, comprehensive)
2. **Decide approach**: Embedded (simpler) or Microservice (scalable)
3. **Start implementation**: Use the code examples in the design doc
4. **Test locally**: Docker compose setup provided in design doc
5. **Deploy incrementally**: Staging → Production

---

## Quick Reference: Key Decisions

| Question | Decision |
|----------|----------|
| Can it be a microservice? | ✅ Yes, strongly recommended |
| Does it fit goop2-services pattern? | ✅ Yes, exactly |
| Will relay need changes? | ✅ Minimal - internal API only |
| Can it scale horizontally? | ✅ Yes - load balance N instances |
| Is it production-ready? | ✅ After implementation + testing |
| Timeline? | ~2 weeks for skilled Go developer |

---

## Important: Plugin Architecture (Static Config)

The bridge follows the **same plugin architecture as other goop2-services**:
- **NOT dynamic discovery** - everything is statically configured
- **NOT service registry** - services are not auto-discovered
- Just like credits, email, registration, templates services
- Each has its own config.json that declares dependencies

See `./PLUGIN_ARCHITECTURE_CLARIFICATION.md` for the correct model.

---

## Document Structure

The full design document (`./HTTPS_BRIDGE_DESIGN.md`) is organized as:

1. **Microservice Architecture** (this decision)
2. **Core Concepts** (VPeer, gateway, relay)
3. **Architecture Diagrams** (visual flows)
4. **Virtual Peer System** (lifecycle, state machine)
5. **API Design** (REST endpoints, WebSocket)
6. **Protocol Integration** (how it plugs into existing systems)
7. **Client Flows** (mobile, web, desktop)
8. **Implementation Details** (Go code patterns)
9. **Data Structures** (Go types + schema)
10. **Real Examples** (chat, calls, group join)
11. **Edge Cases** (reconnection, timeouts, failures)
12. **Security** (auth, rate limiting, TLS)
13. **Microservice Deployment** (systemd, Docker, NGINX)
14. **Deployment Checklist** (step-by-step)
15. **Configuration** (both embedded and microservice configs)

---

**Recommendation**: Start with the embedded approach for MVP, then migrate to microservice when mobile client adoption requires scaling.
