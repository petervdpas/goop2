# HTTPS Bridge Design Documentation Index

**Created**: 2026-03-02
**Status**: Complete - Ready for implementation

---

## 🎯 Critical Clarification: Plugin Architecture

The bridge is **NOT a special plugin** - it's **just another service** in goop2-services, following the **static config pattern** (no dynamic discovery):

- ✅ Bridge declares relay URL in `https/config.json`
- ✅ Relay declares bridge URL in its config
- ✅ All inter-service communication is HTTP (static endpoints)
- ✅ Same pattern as credits, email, registration, templates

See **`./PLUGIN_ARCHITECTURE_CLARIFICATION.md`** for details.

---

## Quick Navigation

This design is documented across three files with different purposes:

### 1. **HTTPS_vs_HTTP_INTERNAL.md** 📋
**Read this first if you care about security/networking**

Explicit, detailed analysis of when to use HTTP vs HTTPS:
- Why internal communication uses HTTP (not HTTPS)
- Threat models and trust boundaries
- Network topology and encryption
- Cryptographic perspective
- When you WOULD use HTTPS internally
- Comparison with industry standards (Kubernetes, AWS, gRPC)
- **Perfect for**: Security reviews, compliance questions, understanding trade-offs

**Key takeaway**:
- ✅ External (Client → Bridge): HTTPS (encryption required)
- ✅ Internal (Bridge → Relay): HTTP (trusted network, no encryption needed)

---

### 2. **MICROSERVICE_ASSESSMENT.md** 📊
**Read this second for the executive summary**

Quick reference guide:
- Can the bridge be a microservice? **✅ YES**
- Why it works as microservice
- Trade-offs table (embedded vs. microservice)
- Effort estimate (~1,600 LOC, 10 days)
- File structure
- Quick decision matrix
- Next steps

**Perfect for**: Managers, architects, "convince me it's worth doing this"

---

### 3. **PLUGIN_ARCHITECTURE_CLARIFICATION.md** 🔌
**Read this to understand the plugin model**

How the bridge fits into goop2-services plugin architecture:
- Static config pattern (like credits, email, registration, templates)
- No dynamic discovery or service registry
- Each service declares its dependencies in config.json
- Simple, predictable, consistent

**Perfect for**: Understanding the architecture, making deployment decisions

---

### 4. **SWAGGER_SDK_GENERATION.md** 🔄
**Read this for client SDK generation (industry standard)**

Auto-generate client SDKs from OpenAPI spec:
- Define bridge API in OpenAPI/Swagger
- Auto-generate clients: JavaScript, Swift, Kotlin, Python, etc.
- Swagger is single source of truth
- Clients use generated SDK (not manual HTTP)
- Same approach as AWS, Google, Azure, Stripe, GitHub

**Perfect for**: Developers implementing client SDKs, understanding API contract

---

### 5. **HTTPS_BRIDGE_DESIGN.md** 📗
**Read this fifth for complete implementation details**

Comprehensive specification:
- Core concepts (VPeer, HTTPS Gateway, protocol bridge)
- Architecture diagrams (detailed flows)
- Virtual peer lifecycle & state machine
- Complete REST API specification (every endpoint)
- Protocol integration (how it plugs into group/realtime)
- Client flows (mobile, web, desktop)
- Go code patterns (data structures, methods)
- Data structures + SQL schema
- Real examples with step-by-step timelines
- Edge cases (reconnection, timeouts, buffering, cleanup)
- Security considerations
- Microservice deployment (systemd, Docker, NGINX, load balancing)
- Configuration examples (both embedded and microservice)
- Deployment checklist

**Perfect for**: Developers implementing the feature, detailed code review

---

## Reading Roadmap

### For Different Roles

#### Security/Compliance Officer
1. Read: `./HTTPS_vs_HTTP_INTERNAL.md` (understand the security model)
2. Read: `./HTTPS_BRIDGE_DESIGN.md` → "Security Considerations" section
3. Ask questions about threat model, encryption, compliance requirements

#### Technical Architect / Decisions Maker
1. Read: `./MICROSERVICE_ASSESSMENT.md` (is it viable?)
2. Read: `./HTTPS_vs_HTTP_INTERNAL.md` (understand internal protocol choice)
3. Read: `./HTTPS_BRIDGE_DESIGN.md` → "Microservice Architecture" section

#### Backend Developer (Implementation)
1. Read: `./MICROSERVICE_ASSESSMENT.md` (understand the big picture)
2. Read: `./HTTPS_BRIDGE_DESIGN.md` → "Implementation Details" & "Data Structures"
3. Read: `./HTTPS_BRIDGE_DESIGN.md` → "Deployment Checklist"
4. Reference code examples throughout while coding

#### DevOps/Infrastructure
1. Read: `./HTTPS_vs_HTTP_INTERNAL.md` (understand protocol choices)
2. Read: `./HTTPS_BRIDGE_DESIGN.md` → "Microservice Deployment" section
3. Read: `./HTTPS_BRIDGE_DESIGN.md` → "Configuration" appendix

#### Product Manager / Leadership
1. Read: `./MICROSERVICE_ASSESSMENT.md` (viability + effort)
2. Skim: Architecture diagrams in `./HTTPS_BRIDGE_DESIGN.md`
3. Ask: Timeline, resource requirements, risks

---

## Document Structure

### HTTPS_vs_HTTP_INTERNAL.md (Explicit Protocol Decisions)

```
├─ Executive Summary
├─ Network Topology & Trust Boundaries
├─ Detailed Comparison (External vs Internal)
├─ Authentication Mechanisms
├─ Encryption: Where It Actually Matters
├─ Cryptographic Perspective
├─ Comparison with Industry Standards
├─ When You WOULD Want HTTPS Internally
├─ Security Model: Explicit Assumptions
├─ Implementation Details (config examples)
├─ Testing & Debugging
├─ Performance Impact
├─ Decision Matrix
└─ Conclusion (Protocol summary table)
```

### MICROSERVICE_ASSESSMENT.md (Executive Summary)

```
├─ Executive Summary
├─ Quick Comparison (embedded vs microservice)
├─ Why Microservice Works
├─ Architecture Overview
├─ Implementation Checklist (high-level)
├─ File Structure
├─ Trade-offs Summary
├─ Effort Estimate
├─ Next Steps
└─ Quick Reference (decision table)
```

### HTTPS_BRIDGE_DESIGN.md (Complete Specification)

```
├─ Overview
├─ Microservice Architecture
│  ├─ Overview: Bridge as Standalone Service
│  ├─ Deployment Options (A vs B)
│  ├─ Microservice Layout
│  ├─ Communication Protocol
│  ├─ Relay Changes
│  └─ Assessment: Microservice Viability
├─ Core Concepts (VPeer, Gateway, Relay)
├─ Architecture (detailed diagrams)
├─ Virtual Peer System (lifecycle, state machine)
├─ API Design (REST endpoints, WebSocket, SSE)
├─ Protocol Integration (with group/realtime)
├─ Client Flows (mobile, web, desktop)
├─ Implementation Details (Go code examples)
├─ Data Structures (Go types, SQL schema)
├─ Examples (chat, calls, group join - step by step)
├─ Edge Cases & Reliability (reconnection, timeouts, etc.)
├─ Security Considerations
├─ Microservice Deployment (systemd, Docker, NGINX, load balancing)
├─ Deployment Checklist
├─ Appendix: Configuration (YAML + JSON examples)
└─ Future Enhancements
```

---

## Key Design Decisions Explained

### Decision 1: HTTP for Internal API (Not HTTPS)

**Question**: Why not HTTPS between bridge and relay?
**Answer**: See `./HTTPS_vs_HTTP_INTERNAL.md` - internal communication stays on trusted network (data center), firewall-protected, no exposure to internet. HTTPS adds CPU cost with zero security benefit.

### Decision 2: Microservice Architecture (Separate Service)

**Question**: Why split bridge from relay?
**Answer**: See `./MICROSERVICE_ASSESSMENT.md` - enables horizontal scaling (multiple bridge instances), fault isolation (bridge crash ≠ P2P crash), independent deployment. Stateless design makes it perfect for microservices.

### Decision 3: Virtual Peers (Not Message Proxying)

**Question**: Why create "virtual peers" instead of just proxying messages?
**Answer**: See `./HTTPS_BRIDGE_DESIGN.md` → "Core Concepts" - virtual peers are first-class network participants, work seamlessly with existing group/realtime protocols. HTTPS clients see no difference from direct P2P peers.

### Decision 4: WebSocket + Bearer Token (Not gRPC)

**Question**: Why REST + WebSocket instead of gRPC?
**Answer**:
- REST easier for mobile SDKs (HTTP libraries ubiquitous)
- WebSocket simpler than gRPC streaming
- JSON more familiar than Protobuf
- TLS termination at load balancer simplifies architecture

---

## Cross-Document References

### When reading HTTPS_BRIDGE_DESIGN.md...

- **"Is this for production?"** → See MICROSERVICE_ASSESSMENT.md (trade-offs)
- **"Why HTTP for internal?"** → See HTTPS_vs_HTTP_INTERNAL.md (detailed security analysis)
- **"Can this scale?"** → See MICROSERVICE_ASSESSMENT.md (horizontal scaling)
- **"Is there a simpler way?"** → See MICROSERVICE_ASSESSMENT.md (Option A: embedded)

### When reading MICROSERVICE_ASSESSMENT.md...

- **"How does it actually work?"** → See HTTPS_BRIDGE_DESIGN.md (complete spec)
- **"What about the HTTP vs HTTPS thing?"** → See HTTPS_vs_HTTP_INTERNAL.md (detailed)
- **"Show me the code"** → See HTTPS_BRIDGE_DESIGN.md → Implementation Details

### When reading HTTPS_vs_HTTP_INTERNAL.md...

- **"How is this used?"** → See HTTPS_BRIDGE_DESIGN.md → Microservice Deployment
- **"What's the big picture?"** → See MICROSERVICE_ASSESSMENT.md
- **"Show me the architecture"** → See HTTPS_BRIDGE_DESIGN.md (diagrams)

---

## Implementation Roadmap

### Phase 1: Design Review (This Documentation)
- ✅ Technical specification complete (HTTPS_BRIDGE_DESIGN.md)
- ✅ Microservice viability confirmed (MICROSERVICE_ASSESSMENT.md)
- ✅ Security/protocol decisions documented (HTTPS_vs_HTTP_INTERNAL.md)

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

**Total: ~10 days** (see MICROSERVICE_ASSESSMENT.md)

---

## FAQ

### Q: Can I start with embedded (simpler) and migrate to microservice later?
**A**: Yes. See MICROSERVICE_ASSESSMENT.md → Recommendation. Code is nearly identical, just move from relay package to separate service.

### Q: Why not just HTTP for everything (client + internal)?
**A**: Clients cross untrusted internet. See HTTPS_vs_HTTP_INTERNAL.md → External API section - clients need TLS encryption.

### Q: What if I need HTTPS internally for compliance?
**A**: See HTTPS_vs_HTTP_INTERNAL.md → "When You WOULD Want HTTPS Internally" - examples given. Easy to add if needed, but not required for our use case.

### Q: How many virtual peers can one bridge handle?
**A**: See HTTPS_BRIDGE_DESIGN.md → "Data Structures" - designed for 10,000 concurrent vpeers per instance. Load balance across 3+ instances for production.

### Q: Can local mDNS peers still work directly with each other?
**A**: Yes. See HTTPS_BRIDGE_DESIGN.md → "What Stays the Same" - local P2P is unchanged. Bridge only handles remote (HTTPS) clients.

### Q: What happens if the bridge crashes?
**A**: See HTTPS_BRIDGE_DESIGN.md → "Edge Cases" - HTTPS clients disconnect gracefully. Relay/local P2P unaffected. Advantage of microservice architecture.

---

## Quick Stats

| Metric | Value |
|--------|-------|
| **Total documentation** | 5,000+ lines |
| **Code examples** | 50+ |
| **Diagrams** | 10+ |
| **Real examples** | 5 (chat, calls, group join, reconnection, load balancing) |
| **Estimated implementation** | 1,600 LOC |
| **Estimated time** | 10 days |
| **Threat model coverage** | Comprehensive |
| **Security considerations** | Deep |
| **Scalability** | Horizontal (N instances) |

---

## Next Steps

1. **Security review** (stakeholders read HTTPS_vs_HTTP_INTERNAL.md)
2. **Architecture approval** (team agrees on microservice approach)
3. **Start implementation** (follow HTTPS_BRIDGE_DESIGN.md)
4. **Deploy** (follow Microservice Deployment section)

---

**All documents are in the root of the goop2 project:**
- `./HTTPS_BRIDGE_DESIGN.md` - Complete specification
- `./MICROSERVICE_ASSESSMENT.md` - Executive summary
- `./HTTPS_vs_HTTP_INTERNAL.md` - Protocol decisions
- `./DESIGN_DOCS_INDEX.md` - This file (navigation)
