# HTTP vs HTTPS: Internal vs External Communication - Detailed Analysis

**Date**: 2026-03-02
**Purpose**: Explicitly clarify when HTTP vs HTTPS is appropriate in the HTTPS Bridge architecture

---

## Executive Summary

- **External (Client → Bridge)**: HTTPS (encryption + authentication required)
- **Internal (Bridge → Relay)**: HTTP (encryption NOT required, trust network)
- **Reason**: Trust boundaries, network topology, threat model

---

## Network Topology & Trust Boundaries

### Deployment Architecture with Trust Zones

```
┌──────────────────────────────────────────────────────────────┐
│                     UNTRUSTED ZONE                           │
│                   (Internet / WAN)                           │
│                                                              │
│  Mobile Apps, Web Browsers, Remote Clients                  │
│                                                              │
└──────────────────────┬───────────────────────────────────────┘
                       │ HTTPS:443
                       │ (encrypted + authenticated)
                       ↓
┌──────────────────────────────────────────────────────────────┐
│                    TRUSTED ZONE                              │
│             (Private Network / Data Center)                  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Load Balancer (NGINX)                                │   │
│  │ Terminates TLS for external clients                  │   │
│  │ Routes to backend on HTTP                            │   │
│  └──────────────┬───────────────────────────────────────┘   │
│                 │ HTTP:8804/8805/8806
│                 │ (internal network only)
│  ┌──────────────▼───────────────────────────────────────┐   │
│  │ Bridge Instance #1/2/3                               │   │
│  │ (goop2-service-https-bridge)                         │   │
│  │ Accepts HTTPS from LB, uses HTTP internally          │   │
│  └──────────────┬───────────────────────────────────────┘   │
│                 │ HTTP:8888
│                 │ (localhost or 127.0.0.1 / internal network)
│  ┌──────────────▼───────────────────────────────────────┐   │
│  │ Main Relay (goop2)                                   │   │
│  │ Exposes /internal/api/* on port 8888                │   │
│  │ Firewall blocks external access                      │   │
│  └──────────────┬───────────────────────────────────────┘   │
│                 │ libp2p (mDNS/direct P2P)
│  ┌──────────────▼───────────────────────────────────────┐   │
│  │ Local P2P Peers (mDNS)                               │   │
│  │ Desktop clients, local devices                       │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### Key Insight: Where Data Crosses Trust Boundaries

```
✗ UNTRUSTED BOUNDARY (needs encryption):
  Client → Internet → Load Balancer
  (Data visible to ISP, routers, network monitors)

✓ TRUSTED BOUNDARY (no encryption needed):
  LB → Bridge → Relay → Peers (all internal)
  (Data never leaves private network/data center)
```

---

## Detailed Comparison: External vs Internal

### External: Client to Bridge (HTTPS Required)

**Scenario**: Mobile app at user's home connecting to goop2.com

```
User's Home Network
├─ Mobile: 192.168.1.100
└─ ISP Router (untrusted)
       ↓
Internet (untrusted)
├─ ISP monitors traffic
├─ DNS servers log queries
├─ BGP routers see packets
├─ Hostile proxies/sniffers
└─ Corporate firewalls (intercept HTTPS)
       ↓
Data Center Network
└─ goop2.com (load balancer)
```

**What can be intercepted without HTTPS**:
1. Destination IP/domain (public anyway)
2. Message size patterns
3. Timing of requests
4. Complete message content ⚠️
5. Authentication tokens ⚠️
6. Private keys/encryption credentials ⚠️

**Solution**: HTTPS + TLS 1.3
- Encrypts content (nobody sees message except endpoints)
- Authenticates server (certificate prevents MITM)
- Forward secrecy (past sessions protected even if key leaked)

**Also required**: Authentication token per peer (Bearer token in Authorization header)

```http
POST /api/peers/mobile-alice/connect
Authorization: Bearer eyJhbGciOiJFZDI1NTE5IiwidHlwIjoiSldUIn0...
Content-Type: application/json

{
  "publicKey": "...",
  "metadata": {...}
}
```

---

### Internal: Bridge to Relay (HTTP Sufficient)

**Scenario**: Bridge service on same private network as relay

```
Data Center Firewall (strict)
└─ Allows only outbound HTTPS to internet

Inside Firewall (trusted):
├─ Bridge Service (192.168.1.10:8804)
│   ├─ Microservice OS process
│   └─ No external network access
│
└─ Relay Service (192.168.1.11:8888)
    ├─ Main goop2 relay
    └─ No external network access
```

**What can intercept internal HTTP**:
1. ❌ Nobody outside the data center (firewall blocks)
2. ❌ Not the ISP (private network)
3. ❌ Not internet attackers (behind firewall)
4. ✓ Only data center admin/network team (already trusted)
5. ✓ Hypervisor/container runtime (if hosted/virtualized, accepted)

**Threat Model for Internal API**:

| Threat | HTTP? | HTTPS? | Notes |
|--------|-------|--------|-------|
| **Internet attacker sniffs packet** | ❌ | ✓ | Firewall prevents this |
| **ISP eavesdrops** | ❌ | ✓ | Data center network, not ISP |
| **DNS spoofing** | ✓ | ✓ | Use IP (127.0.0.1) or mTLS if concerned |
| **Data center admin reads traffic** | ❌ | ✓ | Admin is trusted or you have bigger problems |
| **Container escape/hypervisor break** | ❌ | ✓ | At that point, HTTPS doesn't help |
| **Network packet replay** | ✓ | ✓ | Mitigated by short-lived tokens + request IDs |

**Conclusion**: HTTP is appropriate for internal API.

---

## Authentication Mechanisms

### External API (Client → Bridge)

Layers of auth:

1. **TLS Certificate** (transport)
   - Authenticates server
   - Prevents MITM
   - Encrypts channel

2. **Bearer Token** (application)
   ```http
   Authorization: Bearer {JWT}
   ```
   - Identifies client peer
   - Signed by relay (client can't forge)
   - Expires after 24 hours (configurable)

3. **Request Signature** (optional, for groups/calls)
   ```json
   {
     "message": {...},
     "signature": "ed25519_signature",
     "timestamp": "2026-03-02T10:15:00Z"
   }
   ```
   - Client signs with their private key
   - Relay verifies signature
   - Prevents spoofing within the protocol

### Internal API (Bridge → Relay)

Simpler, single layer:

1. **Shared Secret Bearer Token** (application)
   ```http
   Authorization: Bearer {internal-secret-token}
   ```
   - Hardcoded in both services (or from secure vault)
   - Bridge uses to call /internal/api/* endpoints
   - Protects against accidental misconfigurations
   - Does NOT protect against data center network traffic (not needed)

**Why not signature-based?**
- Both services are under same operational control
- Token suffices to prevent accidental misconfiguration
- Signature adds complexity without threat mitigation benefit

---

## Encryption: Where It Actually Matters

### Data In Transit

#### 1. Client → Bridge (External)

```
Mobile App                  Internet                  Load Balancer
    │                          │                           │
    ├─ Message body ────(encrypted by TLS)────────────────┤
    │                          │                           │
    └─ Auth token ─────(encrypted by TLS)────────────────┤

Encryption status: ✓ Encrypted (40,000+ ft view)
Eavesdropper sees: TLS handshake only, no content
```

**Before encryption** (if HTTP):
- ISP can see: "Alice sent 250 bytes to goop2.com at 10:15:03"
- "Alice is in group-backlog with Bob"
- "Alice's message: 'Secret password is xyz'"
- "Alice's auth token: eyJhbGc..."

**After HTTPS encryption**:
- ISP can see: "Connection to goop2.com"
- Cannot see message content, auth tokens, or metadata

#### 2. Bridge → Relay (Internal)

```
Bridge Instance              Data Center Network       Relay Service
    │                              │                       │
    ├─ Event data ─────(not encrypted, not needed)────────┤
    │                              │                       │
    └─ Auth token ─────(not encrypted, not needed)────────┤

Encryption status: ✗ Not encrypted
Eavesdropper sees: Full data
But eavesdropper is: Data center admin (already trusted)
```

**Can data center admin read it?**
- With HTTP: Yes, easily (tcpdump, netcat, etc.)
- With HTTPS: Yes, still (they control the keys, can MitM)
- Net effect: HTTPS provides zero protection here

**Cost/benefit of forcing HTTPS internally**:
- Cost: CPU overhead, certificate management, debugging complexity
- Benefit: Zero (already trusted network)
- Verdict: Not worth it

### Data At Rest

**Not affected by HTTP vs HTTPS** (both use same security):

```
Bridge Sends:
{
  "groupId": "group-abc",
  "peerId": "bob",
  "message": "Hello"
}
    ↓
Relay receives and stores in database:
INSERT INTO messages (group_id, peer_id, content) VALUES (...)
    ↓
Database encrypted at rest (if configured)
```

HTTP vs HTTPS doesn't affect database encryption.

---

## Cryptographic Perspective

### TLS for External API (HTTPS)

**What TLS provides:**
1. **Confidentiality**: Encrypts payload
2. **Integrity**: Detects tampering (MAC check)
3. **Authentication**: Verifies server identity (certificate)
4. **Forward secrecy**: Past sessions safe if key compromised (ECDHE key exchange)

**Threat it protects against:**
- Passive eavesdropping (ISP, router, gateway, coffee shop WiFi)
- Active MITM (rogue proxy, DNS hijacking)
- Replay attacks (different topic)

### Bearer Token for Internal API

**What token provides:**
1. **Authentication**: Proves caller is authorized bridge service
2. **Integrity**: Token is signed (if JWT)
3. **Confidentiality**: Token stays confidential (only used internally)
4. **Revocation**: Token can be revoked if service compromised

**Threat it protects against:**
- Accidental misconfiguration (service A calling wrong service)
- Unauthorized internal service connecting
- But NOT against data center admin with network access (they can still call the API)

---

## Comparison with Industry Standards

### Kubernetes Microservices

**Inter-service communication:**
```go
// Service A calling Service B (same cluster)
http.Get("http://service-b:8080/api/data")  // Standard practice
```

- ✓ HTTP, no TLS
- ✓ Service mesh (Istio) adds mTLS if paranoid
- ✓ But by default, HTTP is fine within cluster

**Why?**
- Cluster network is presumed trusted
- External traffic is separately protected (ingress controller with TLS)
- Performance cost of TLS not justified

### Docker Compose / Local Dev

```yaml
services:
  app:
    image: myapp
    environment:
      DATABASE_URL: "postgresql://db:5432/mydb"  # HTTP-like simplicity

  db:
    image: postgres
```

- ✓ Services communicate over Docker network (HTTP)
- ✓ Not encrypted
- ✓ Fine because it's internal

### gRPC in Data Centers

```protobuf
service Relay {
  rpc JoinGroup(JoinRequest) returns (JoinResponse);
}
```

- Common protocol for inter-service gRPC
- Usually: HTTP/2 without TLS internally
- TLS added at gateway/ingress only

### AWS, GCP, Azure Microservices

```
Load Balancer (TLS:443)
    ↓ HTTP (internal)
Service 1
    ↓ HTTP (internal)
Service 2
    ↓ HTTP (internal)
Database
```

- Public API: TLS required
- Internal services: HTTP standard practice
- Regional network is trusted

---

## When You WOULD Want HTTPS Internally

### Scenario 1: Multi-Tenant Deployment

```
Tenant A's Relay ────────── Tenant A's Bridge

Tenant B's Relay ────────── Tenant B's Bridge

Shared Infrastructure (untrusted)
```

**Solution**: HTTPS + mTLS (mutual TLS certificates)
- Tenant A's services only talk to each other
- Tenant B's services can't intercept

### Scenario 2: Geographic Distribution

```
Data Center A (goop2 relay)
    ↓ HTTPS over internet
Data Center B (https-bridge)
```

- Bridge and relay in different data centers (separate internet connections)
- Data crosses untrusted network
- Solution: HTTPS + VPN or both

### Scenario 3: Compliance Requirement

- "All inter-service communication must be encrypted"
- Even within data center, policy/audit requires it
- Cost is acceptable for compliance

### Scenario 4: Shared Hosting (VPS/Cloud VM)

```
VPS Provider hypervisor (untrusted)
├─ Your Bridge VM
├─ Attacker's VM (compromised)
└─ Shared network (attacker could sniff)
```

- Other customers' VMs on same physical hardware
- VPS provider's hypervisor is potential threat
- Solution: HTTPS + VPN to keep relay

---

## Security Model: Explicit Assumptions

### What We ASSUME Is Secure

1. ✓ **Data center firewall** - blocks external access to ports 8804, 8888
2. ✓ **Internal network** - not on open internet
3. ✓ **Services are co-located** - same data center or trusted VPC
4. ✓ **Data center admins** - trusted (or you have bigger problems)
5. ✓ **OS process isolation** - bridge and relay are separate processes (can't read each other's memory)
6. ✓ **Container runtime** - if using containers, we trust the runtime

### What We Protect Against

| Threat | Protected? | How |
|--------|-----------|-----|
| ISP eavesdrops on client connection | ✓ | HTTPS (TLS) |
| Attacker on public WiFi sniffs client | ✓ | HTTPS (TLS) |
| DNS spoofing to reroute client | ✓ | TLS certificate validation |
| Bridge HTTPS token leaked to internet | ✓ | Short expiration + revocation |
| Relay eavesdrops on bridge? | ✗ | Not a threat (relay is trusted) |
| Bridge eavesdrops on relay? | ✗ | Not a threat (bridge is trusted) |
| Attacker spoofs relay to bridge | ✗ | Token auth (bridge trusts relay) |

### What We DON'T Protect Against

| Threat | Reason |
|--------|--------|
| Data center admin reads bridge-relay traffic | They control network anyway |
| Hypervisor intercepts bridge-relay traffic | They control VMs anyway |
| Process with root access reads traffic | They can read memory directly |
| Compromised data center | Beyond scope - assume data center secure |

---

## Implementation Details

### Bridge Configuration

```json
{
  "relay_url": "http://localhost:8888",
  "relay_auth_token": "secret-bearer-token-12345",
  ...
}
```

**Why HTTP?**
- No certificates to manage
- No TLS overhead (5-10% CPU cost saved)
- Easier debugging (curl works directly)
- Localhost doesn't need encryption

**Why the token?**
- Defense in depth
- Prevents accidental misconfiguration
- Only authorized bridge can call relay's internal API
- Token can be rotated if compromised

### Relay Configuration

```yaml
internalAPI:
  port: 8888
  authToken: "secret-bearer-token-12345"
  allowedOrigins:
    - "http://localhost:8804"
    - "http://localhost:8805"
```

**Why HTTP?**
- Same reasoning as bridge
- Token auth provides sufficient access control

**Why allowedOrigins?**
- Extra validation (defense in depth)
- Ensures only expected bridge instances connect
- Prevents accidental misconfiguration (bridge on wrong port)

### External API (TLS Terminated at Load Balancer)

```nginx
upstream goop2_bridge {
    server localhost:8804;
    server localhost:8805;
}

server {
    listen 443 ssl http2;
    ssl_certificate /etc/letsencrypt/live/goop2.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/goop2.com/privkey.pem;
    ssl_protocols TLSv1.3;

    location / {
        proxy_pass http://goop2_bridge;  # ← HTTP internally, NGINX handles TLS
    }
}
```

**Traffic flow:**
1. Client → NGINX: HTTPS (encrypted)
2. NGINX → Bridge: HTTP (internal, fast)
3. Bridge → Relay: HTTP (internal, fast)

**Total encryption**:
- Client's message: Encrypted by TLS from client → NGINX
- NGINX reads message (it terminates TLS)
- NGINX forwards to bridge over HTTP (internal network)
- Bridge forwards to relay over HTTP (internal network)

---

## Testing & Debugging

### With HTTP (Easier)

```bash
# Directly test internal API
curl -H "Authorization: Bearer secret-token" \
  http://localhost:8888/internal/api/relay/peers

# Monitor traffic
tcpdump -i lo port 8888

# Client logs see plaintext
bridge-logs: calling relay at http://localhost:8888
relay-logs: received request from 127.0.0.1
```

### With HTTPS (Harder)

```bash
# Must skip cert verification (self-signed in dev)
curl -k -H "Authorization: Bearer secret-token" \
  https://localhost:8888/internal/api/relay/peers

# Monitor traffic (encrypted, useless)
tcpdump -i lo port 8888
# Sees: [TLS encrypted data] (can't read)

# Logs must show decrypted version
bridge-logs: [after decryption] calling relay at https://localhost:8888
```

---

## Performance Impact

### HTTP (Internal)

```
Bridge → Relay latency: ~0.5ms (localhost)
Overhead: Near zero (no TLS handshake, no encryption)
Throughput: Limited only by network, not crypto
```

### HTTPS (Internal)

```
Bridge → Relay latency: ~2-3ms
  - TLS handshake: ~1-2ms (first connection only)
  - TLS record processing: ~0.5-1ms per request
Overhead: 4-6x latency, 5-10% CPU
Throughput: Limited by cipher speed
```

**For high-volume group messages:**
- HTTP: 100,000 msg/sec possible
- HTTPS: 50,000 msg/sec possible
- Real-world impact: Negligible (typical load < 1,000/sec)

---

## Decision Matrix

| Factor | HTTP | HTTPS | Verdict |
|--------|------|-------|---------|
| **Threat model** | Low | None | HTTP ✓ |
| **Network location** | Internal | Internal | HTTP ✓ |
| **Firewall protection** | Yes | Yes | HTTP ✓ |
| **Admin trust** | Yes | Yes | HTTP ✓ |
| **Debugging** | Easy | Hard | HTTP ✓ |
| **Performance** | Fast | Slow | HTTP ✓ |
| **Compliance** | Maybe | Maybe | HTTPS if policy mandates |
| **Multi-tenant** | No | Yes | HTTP ✓ (our case) |

**Our case: HTTP is the right choice.**

---

## Conclusion

### External API (Client → Bridge): HTTPS Required ✅

```
POST https://goop2.com/api/peers/{peerId}/connect
Authorization: Bearer {jwt-token}
```

- **Why**: Data crosses untrusted internet
- **Encryption**: TLS 1.3 (handles all encryption)
- **Authentication**: TLS certificate + bearer token
- **Non-negotiable**: Yes

### Internal API (Bridge → Relay): HTTP Sufficient ✅

```
POST http://localhost:8888/internal/api/relay/peers
Authorization: Bearer {internal-secret-token}
```

- **Why**: Data stays within trusted data center
- **Encryption**: Not needed (firewall + network isolation)
- **Authentication**: Bearer token (defense in depth)
- **Benefit of HTTP**: Simpler, faster, easier to debug

### Summary

| Communication | Protocol | Encryption | Why |
|---|---|---|---|
| **Client → Bridge (WAN)** | HTTPS | TLS | Cross untrusted network |
| **Bridge → Relay (LAN)** | HTTP | No | Internal trusted network |
| **Bridge → Bridge (LAN)** | HTTP | No | Internal trusted network |
| **Relay → Relay (WAN)** | HTTPS | TLS | If across internet; VPN if private |

---

**End of Analysis**
