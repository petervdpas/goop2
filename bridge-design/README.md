# HTTPS Bridge Design Documentation

Complete design documentation for implementing an HTTPS Bridge service in goop2-services to enable mobile, web, and remote clients to participate in the P2P network as virtual peers.

**Status**: ✅ Complete - Ready for implementation
**Date**: 2026-03-02
**Total Documentation**: 5,000+ lines, 50+ code examples, 10+ diagrams

---

## Start Here

### 1️⃣ **NEW** - Plugin Architecture (5 min read)
**File**: [`./PLUGIN_ARCHITECTURE_CLARIFICATION.md`](./PLUGIN_ARCHITECTURE_CLARIFICATION.md)

**What**: The bridge is just another service in goop2-services (like credits, email, registration, templates) - no dynamic discovery, just static config.

**Perfect for**: Understanding how the bridge fits into the existing architecture.

---

### 2️⃣ HTTP vs HTTPS Protocol (10 min read)
**File**: [`./HTTPS_vs_HTTP_INTERNAL.md`](./HTTPS_vs_HTTP_INTERNAL.md)

**What**: Detailed explanation of why:
- ✅ External (Client → Bridge): **HTTPS** (encryption required)
- ✅ Internal (Bridge → Relay): **HTTP** (trusted network, no encryption needed)

**Perfect for**: Security reviews, understanding threat model, compliance questions.

---

### 3️⃣ Executive Summary (5 min read)
**File**: [`./MICROSERVICE_ASSESSMENT.md`](./MICROSERVICE_ASSESSMENT.md)

**What**: Quick reference including:
- Can it be a microservice? **✅ YES**
- Why it works
- Trade-offs (embedded vs microservice)
- Effort estimate (~10 days, 1,600 LOC)
- Next steps

**Perfect for**: Managers, architects, decision makers.

---

### 4️⃣ Swagger & SDK Generation (Industry Standard)
**File**: [`./SWAGGER_SDK_GENERATION.md`](./SWAGGER_SDK_GENERATION.md)

**What**: Auto-generate client SDKs from OpenAPI spec:
- Define bridge API once in OpenAPI/Swagger
- Auto-generate clients for JavaScript, Swift, Kotlin, Python, etc.
- Swagger is single source of truth
- Industry standard (AWS, Google, Azure, Stripe, GitHub all do this)

**Perfect for**: Implementing client SDKs, understanding API contract generation.

---

### 5️⃣ Complete Specification (Deep dive)
**File**: [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md)

**What**: 85KB comprehensive spec including:
- Core concepts (VPeer, gateway, protocol bridge)
- Architecture diagrams
- Virtual peer lifecycle & state machine
- Complete REST API specification
- Protocol integration
- Client flows (mobile, web, desktop)
- Go code patterns
- Real examples with timelines
- Edge cases & reliability
- Security considerations
- Microservice deployment (systemd, Docker, NGINX)
- Configuration examples
- Deployment checklist

**Perfect for**: Developers implementing the feature, detailed code review.

---

### 5️⃣ Navigation Guide
**File**: [`./DESIGN_DOCS_INDEX.md`](./DESIGN_DOCS_INDEX.md)

**What**: Cross-document index and reading roadmaps by role.

**Perfect for**: Finding specific information quickly.

---

## Quick Facts

| Metric | Value |
|--------|-------|
| **Microservice Viable?** | ✅ YES |
| **Plugin Architecture?** | ✅ Static config (like other services) |
| **Protocol: External** | HTTPS (TLS 1.3) |
| **Protocol: Internal** | HTTP (trusted network) |
| **Implementation** | ~1,600 LOC |
| **Timeline** | ~10 days |
| **Horizontal Scaling** | ✅ Multiple bridge instances |
| **Stateless?** | ✅ YES (load balanced) |

---

## Key Decision Points

### External API (Client → Bridge)
```
POST https://goop2.com/api/peers/{peerId}/connect
Authorization: Bearer {jwt-token}
```
- **Protocol**: HTTPS (TLS 1.3)
- **Why**: Data crosses untrusted internet
- **Non-negotiable**: Yes

### Internal API (Bridge → Relay)
```
POST http://localhost:8888/internal/api/relay/peers
Authorization: Bearer {internal-secret-token}
```
- **Protocol**: HTTP (not HTTPS)
- **Why**: Internal trusted network, firewall protected
- **Standard Practice**: Yes (Kubernetes, gRPC, AWS all use HTTP internally)

### Service Architecture
```json
// https/config.json (bridge declares relay dependency)
{
  "addr": ":8804",
  "relay_url": "http://localhost:8888",  // Static config
  "relay_auth_token": "secret"
}
```
- **Pattern**: Same as credits, email, registration, templates
- **Discovery**: None (static config)
- **Complexity**: Simple

---

## Implementation Roadmap

### Phase 1: Design Review (Today)
- ✅ Technical specification complete
- ✅ Microservice viability confirmed
- ✅ Security/protocol decisions documented
- ✅ Plugin architecture clarified

### Phase 2: Core Implementation (~5 days)
- Create `https/` package in goop2-services
- Implement VirtualPeerManager + HTTPSGateway
- Implement RelayClient
- Add /internal/api/* to relay
- Local testing with docker-compose

### Phase 3: Integration Testing (~2 days)
- Mobile app → Bridge connection
- Bridge → Relay internal API
- Group joining, messaging, calls
- Load testing (100+ concurrent vpeers)

### Phase 4: Deployment (~2 days)
- Systemd units
- NGINX load balancer config
- TLS certificates
- Production deployment

### Phase 5: Monitoring (~1 day)
- Prometheus metrics
- Alerting rules
- Log aggregation

**Total: ~10 days**

---

## File Organization

```
bridge-design/
├── README.md (this file)
├── PLUGIN_ARCHITECTURE_CLARIFICATION.md  ← Start here
├── HTTPS_vs_HTTP_INTERNAL.md
├── MICROSERVICE_ASSESSMENT.md
├── HTTPS_BRIDGE_DESIGN.md
└── DESIGN_DOCS_INDEX.md
```

---

## Reading by Role

### Security/Compliance Officer
1. [`./PLUGIN_ARCHITECTURE_CLARIFICATION.md`](./PLUGIN_ARCHITECTURE_CLARIFICATION.md) (understand architecture)
2. [`./HTTPS_vs_HTTP_INTERNAL.md`](./HTTPS_vs_HTTP_INTERNAL.md) (understand protocol decisions)
3. [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md) → Security Considerations section

### Technical Architect / Decision Maker
1. [`./PLUGIN_ARCHITECTURE_CLARIFICATION.md`](./PLUGIN_ARCHITECTURE_CLARIFICATION.md) (is it viable?)
2. [`./HTTPS_vs_HTTP_INTERNAL.md`](./HTTPS_vs_HTTP_INTERNAL.md) (understand protocol)
3. [`./MICROSERVICE_ASSESSMENT.md`](./MICROSERVICE_ASSESSMENT.md) (effort & trade-offs)

### Backend Developer (Implementation)
1. [`./PLUGIN_ARCHITECTURE_CLARIFICATION.md`](./PLUGIN_ARCHITECTURE_CLARIFICATION.md) (understand the architecture)
2. [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md) → Implementation Details section
3. [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md) → Deployment Checklist

### DevOps/Infrastructure
1. [`./HTTPS_vs_HTTP_INTERNAL.md`](./HTTPS_vs_HTTP_INTERNAL.md) (understand protocols)
2. [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md) → Microservice Deployment section
3. [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md) → Configuration appendix

### Product/Leadership
1. [`./MICROSERVICE_ASSESSMENT.md`](./MICROSERVICE_ASSESSMENT.md) (viability + effort)
2. Skim architecture diagrams in [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md)
3. Ask: Timeline, resources, risks

---

## Quick Reference

### Virtual Peer Concept
- HTTPS client connects to bridge
- Bridge creates VirtualPeer object for that client
- VirtualPeer is first-class in groups, realtime channels, calls
- Local P2P peers interact with virtual peer like any other peer

### Architecture Summary
```
Clients (HTTPS:443)
    ↓
Load Balancer (terminates TLS)
    ↓
Bridge Instances (HTTP:8804+)
    ↓
Relay (HTTP:8888 internal API)
    ↓
P2P Network (mDNS + libp2p)
```

### Configuration Pattern
- Bridge declares relay dependency in `https/config.json`
- Relay declares bridge endpoint in its config
- All static (no service registry)
- Scales horizontally (multiple bridge instances)

---

## Questions?

- **"Is this viable?"** → See [`./MICROSERVICE_ASSESSMENT.md`](./MICROSERVICE_ASSESSMENT.md)
- **"Why HTTP internally?"** → See [`./HTTPS_vs_HTTP_INTERNAL.md`](./HTTPS_vs_HTTP_INTERNAL.md)
- **"How does it work?"** → See [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md)
- **"How does it fit into goop2-services?"** → See [`./PLUGIN_ARCHITECTURE_CLARIFICATION.md`](./PLUGIN_ARCHITECTURE_CLARIFICATION.md)
- **"Where do I find X?"** → See [`./DESIGN_DOCS_INDEX.md`](./DESIGN_DOCS_INDEX.md)

---

**Ready to implement?** Follow the deployment checklist in [`./HTTPS_BRIDGE_DESIGN.md`](./HTTPS_BRIDGE_DESIGN.md) and reference code examples throughout as you code.

**Have feedback?** All design decisions are documented - if something doesn't make sense, check the rationale in the respective document.
