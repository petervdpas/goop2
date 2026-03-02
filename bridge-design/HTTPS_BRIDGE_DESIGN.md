# HTTPS Bridge & Virtual Peer Architecture

**Date**: 2026-03-02
**Purpose**: Enable mobile, web, and remote clients to participate in goop2's P2P network through an HTTPS relay gateway

---

## Table of Contents

1. [Overview](#overview)
2. [Microservice Architecture](#microservice-architecture)
3. [Core Concepts](#core-concepts)
4. [Architecture](#architecture)
5. [Virtual Peer System](#virtual-peer-system)
6. [API Design](#api-design)
7. [Protocol Integration](#protocol-integration)
8. [Client Flows](#client-flows)
9. [Implementation Details](#implementation-details)
10. [Data Structures](#data-structures)
11. [Examples](#examples)
12. [Edge Cases & Reliability](#edge-cases--reliability)
13. [Security Considerations](#security-considerations)
14. [Microservice Deployment](#microservice-deployment)
15. [Future Enhancements](#future-enhancements)

---

## Overview

The HTTPS Bridge transforms a goop2 relay service into a **peer gateway** that enables clients incapable of direct P2P connections (mobile apps, web browsers, corporate firewalls) to participate in the P2P network as first-class peers.

**Key Innovation**: Instead of proxying messages, the relay **instantiates virtual peer objects** that implement the full goop2 protocol stack on behalf of remote clients. Local mDNS peers interact with these virtual peers identically to how they interact with each other.

### Who Needs This?
- **Mobile apps** (iOS/Android): Can't do mDNS discovery or listen on ports
- **Web browsers** (at goop2.com): Can't initiate libp2p connections
- **Remote desktop clients**: Behind corporate firewalls, NAT, etc.
- **Embedded devices**: Limited network stack

### What Stays the Same?
- Local mDNS peers continue discovering and connecting directly
- Group protocol, realtime channels, WebRTC calls all work transparently
- No changes to the core libp2p protocol layer

---

## Microservice Architecture

### Overview: Bridge as a Standalone Service

The HTTPS Bridge can be deployed as a **standalone microservice** (in the `goop2-services` repository) that communicates with the main goop2 relay via HTTP/gRPC. This allows:

- **Scaling**: Deploy multiple bridge instances with load balancing
- **Isolation**: HTTPS handling separate from P2P protocol logic
- **Resilience**: Bridge crash doesn't affect local P2P network
- **Flexibility**: Use different TLS configs, CDNs, or geographical distribution per bridge instance

### Deployment Options

#### Option A: Embedded in Main Relay (Simpler)
```
┌─────────────────────────┐
│   goop2 Relay           │
│  ┌─────────────────┐    │
│  │ HTTPS Bridge    │ ◄──┼──── Mobile/Web Clients
│  │ (VirtualPeers)  │    │
│  └─────────────────┘    │
│  ┌─────────────────┐    │
│  │ P2P Stack       │ ◄──┼──── Local mDNS Peers
│  │ (Groups, etc.)  │    │
│  └─────────────────┘    │
└─────────────────────────┘
```

**Pros**: Simple, single process, no network overhead
**Cons**: Harder to scale, TLS handling mixed with P2P logic

#### Option B: Microservice in goop2-services (Recommended for Production)
```
┌──────────────────────────┐           ┌─────────────────────────┐
│ goop2-https-bridge       │  HTTP     │   goop2 Relay           │
│ (Microservice)           │  (port)   │                         │
│                          │◄─────────►│  ┌─────────────────┐    │
│ • VirtualPeerManager     │  /tcp/    │  │ P2P Stack       │    │
│ • HTTPS Gateway          │  8888     │  │ (Groups, etc.)  │    │
│ • WebSocket Server       │           │  └─────────────────┘    │
│ • Rate Limiting          │           │                         │
│ • TLS Termination        │           └─────────────────────────┘
└──────────────────────────┘
        ↓
   Mobile/Web Clients (HTTPS)
```

**Pros**: Scalable, isolated, independent lifecycle
**Cons**: Network latency between bridge and relay, added complexity

### Microservice Layout (Plugin Architecture: Static Config)

Bridge is **just another service** like credits, email, registration, templates:

```
goop2-services/
├── cmd/
│   ├── https-bridge-server/
│   │   └── main.go                  (same pattern as email-server, credits-server, etc.)
│   ├── credits-server/
│   ├── email-server/
│   ├── registration-server/
│   └── templates-server/
├── https/                           (plugin service #5)
│   ├── config.go                    # Config struct + LoadConfig()
│   ├── config.json                  # Declares relay_url (static config)
│   ├── server.go                    # Server struct + RegisterRoutes()
│   ├── vpeer_manager.go             # Virtual peer management
│   ├── gateway.go                   # REST/WebSocket routes
│   ├── relay_client.go              # HTTP client to relay
│   ├── websocket.go                 # WebSocket handling
│   └── server_test.go
├── credits/ / email/ / registrations/ / templates/  (other services)
├── go.mod
├── go.sum
├── Makefile                         # build target: https-bridge
└── README.md
```

**No dynamic discovery** - Bridge declares its relay dependency in config (same as credits declares registration + templates dependency):

```json
// https/config.json - static config
{
  "addr": ":8804",
  "relay_url": "http://localhost:8888",  // ← Where relay is (static, like other services)
  "relay_auth_token": "secret"
}
```

See `./PLUGIN_ARCHITECTURE_CLARIFICATION.md` for the complete plugin architecture model.

### Communication Protocol: Bridge ↔ Relay

**CRITICAL DISTINCTION**: Bridge ↔ Relay uses **HTTP (not HTTPS)** because it's internal-only communication. See `./HTTPS_vs_HTTP_INTERNAL.md` for detailed security analysis.

The bridge communicates with the main goop2 relay via **internal HTTP API** (port 8888, localhost/internal network only):

```http
# Relay notifies bridge of peer activity
POST /internal/api/bridge/events
Content-Type: application/json

{
  "type": "peer_broadcast",
  "groupId": "group-abc",
  "peerId": "bob",
  "message": {...},
  "timestamp": "2026-03-02T10:15:00Z"
}

# Bridge queries relay for peer/group info
GET /internal/api/relay/peers?status=active
Authorization: Bearer {internal-token}

[{"peerId": "bob", "metadata": {...}}, ...]

# Bridge joins a group on behalf of virtual peer
POST /internal/api/relay/group/join
Content-Type: application/json

{
  "peerId": "vpeer-alice",    # Synthetic peer ID
  "groupId": "group-abc"
}

Response: {
  "groupId": "group-abc",
  "members": [...],
  "createdAt": "..."
}
```

### Relay Changes for Microservice Bridge

The main goop2 relay needs minimal changes to support an external bridge:

```go
// In relay server
type BridgeConfig struct {
    Enabled       bool
    BridgeURL     string  // "http://localhost:8888"
    InternalToken string  // Secret for /internal/* endpoints
    PeerPrefix    string  // "vp-" for virtual peer IDs
    MaxVPeers     int     // Max virtual peers per bridge (10,000)
}

// When relay receives a message for a virtual peer:
func (r *Relay) SendToVPeer(peerId, message string) error {
    if !strings.HasPrefix(peerId, r.cfg.Bridge.PeerPrefix) {
        // Not a virtual peer, handle normally
        return r.SendToLocalPeer(peerId, message)
    }

    // Forward to bridge service
    return r.bridgeClient.PublishEvent(peerId, message)
}

// Relay exposes internal API for bridge
func (r *Relay) registerInternalAPI(mux *http.ServeMux) {
    mux.HandleFunc("POST /internal/api/bridge/events", r.handleBridgeEvents)
    mux.HandleFunc("GET /internal/api/relay/peers", r.handleRelayPeers)
    mux.HandleFunc("POST /internal/api/relay/group/join", r.handleGroupJoinFromBridge)
    // ... other internal endpoints
}
```

### Assessment: Microservice Viability

**✅ YES - The HTTPS Bridge can be effectively deployed as a microservice in goop2-services.**

#### Why It Works as a Microservice:

1. **Stateless Design**
   - Virtual peers are ephemeral (live only while HTTPS connection active)
   - No persistent per-peer state on bridge
   - State lives in relay (groups, messages, presence)
   - Multiple bridge instances can handle any peer without coordination

2. **Clear Interface Boundaries**
   - Bridge → Relay: HTTP API on port 8888
   - Client → Bridge: HTTPS on port 443 (or :8804 internally)
   - No tight coupling, minimal shared data structures

3. **Follows goop2-services Pattern**
   - Same Config struct + LoadConfig() pattern as credits, email, templates
   - Same Server struct + RegisterRoutes() pattern
   - Same main.go structure
   - Integrates cleanly into existing build/deploy pipeline

4. **Horizontal Scalability**
   - Deploy 3, 5, or 10 bridge instances behind load balancer
   - Each handles independent virtual peers
   - No session stickiness needed (vpeers are HTTP-based, not sticky)
   - Relay becomes bottleneck, not bridge

#### Trade-offs:

| Aspect | Embedded | Microservice |
|--------|----------|--------------|
| **Latency** | ~1ms (in-process) | ~5-10ms (localhost HTTP) |
| **Resource Usage** | Single process | Separate process |
| **Scalability** | Limited to single machine | Unlimited (add more instances) |
| **Complexity** | Simpler | More infrastructure |
| **Deployment** | One systemd unit | Two systemd units + load balancer |
| **Isolation** | None (crash affects P2P) | Full (bridge crash ≠ P2P crash) |

#### Recommendation:

- **Development/Testing**: Start with embedded approach (simpler)
- **Production/Scale**: Use microservice approach (better resilience + horizontal scaling)
- **Path**: Easy to migrate: extract bridge code → move to goop2-services → point relay to external API

#### Implementation Effort:

- **Bridge service code**: ~800 lines (vpeer_manager.go + gateway.go + websocket.go)
- **Relay changes**: ~200 lines (internal API endpoints + bridge config)
- **Infrastructure**: ~100 lines (systemd + NGINX config)
- **Tests**: ~500 lines

**Total effort**: ~1,600 lines of Go code across both repos

---

## Core Concepts

### 1. Virtual Peer (VPeer)

A **virtual peer** is a software object on the relay that:
- Holds a unique `peerId` (string identifier)
- Implements the goop2 protocol interface
- Is created/activated when an HTTPS client connects
- Participates in groups, realtime channels, and protocol flows
- Routes all operations through the HTTPS connection back to the client
- Lives as long as the HTTPS connection is active (with grace period for reconnection)

### 2. HTTPS Gateway

The relay's HTTPS server that:
- Exposes REST endpoints for peer operations
- Manages WebSocket/SSE connections for bidirectional updates
- Translates HTTP requests → virtual peer method calls
- Translates peer events → HTTP responses/WebSocket messages

### 3. Relay Service

The running goop2 service (with embedded HTTP server) that:
- Runs mDNS discovery for local peers
- Hosts the HTTPS gateway
- Manages virtual peers
- Participates in groups as a "relay" or "invisible" role
- Forwards messages between local P2P and remote HTTPS clients

### 4. Protocol Bridge

The layer that:
- Translates virtual peer calls → real group/realtime operations
- Ensures virtual peers are treated as full participants by the protocol
- Handles addressing so local peers know how to reach virtual peers

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    HTTPS Relay Service                          │
│                    (Single goop2 instance)                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌────────────────────────────────────────────────────────┐   │
│  │           HTTPS Gateway (Router + Manager)             │   │
│  │  POST /api/peers/{peerId}/join-group                   │   │
│  │  POST /api/peers/{peerId}/send-message                 │   │
│  │  POST /api/peers/{peerId}/call-initiate                │   │
│  │  WebSocket /ws/peers/{peerId}                          │   │
│  └────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌────────────────────────────────────────────────────────┐   │
│  │         Virtual Peer Manager                           │   │
│  │  • map[peerId]*VirtualPeer                             │   │
│  │  • SessionManager (connection lifetime)                │   │
│  │  • EventBroadcaster (WebSocket/SSE)                    │   │
│  └────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌────────────────────────────────────────────────────────┐   │
│  │  Virtual Peer Instances (one per client)               │   │
│  │  ├─ VPeer("mobile-alice")                              │   │
│  │  ├─ VPeer("web-bob")                                   │   │
│  │  └─ VPeer("remote-charlie")                            │   │
│  │     • Implements Peer interface                         │   │
│  │     • Maintains state (groups, subscriptions, etc.)    │   │
│  │     • Routes calls back to client via HTTPS            │   │
│  └────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌────────────────────────────────────────────────────────┐   │
│  │    goop2 Protocol Layer (Unchanged)                    │   │
│  │  • Group Manager                                       │   │
│  │  • Realtime Manager                                    │   │
│  │  • Chat Manager                                        │   │
│  │  • libp2p Host                                         │   │
│  └────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌────────────────────────────────────────────────────────┐   │
│  │    mDNS Discovery & P2P Connections                    │   │
│  │  • Local peers (mDNS)                                  │   │
│  │  • Direct libp2p connections                           │   │
│  └────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
        ↑                       ↑                       ↑
        │                       │                       │
   ┌────────┐             ┌─────────┐            ┌──────────┐
   │ Mobile │             │  Web    │            │ Remote   │
   │  App   │             │ Browser │            │ Desktop  │
   │(iOS)   │             │(HTTPS)  │            │  (HTTPS) │
   └────────┘             └─────────┘            └──────────┘
      HTTPS                HTTPS                   HTTPS
```

---

## Virtual Peer System

### Virtual Peer Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│ Client connects to relay (HTTPS)                            │
└────────────────────┬────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────────────────────┐
│ POST /api/peers/{peerId}/connect (with auth token)          │
└────────────────────┬────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────────────────────┐
│ Create/Activate VirtualPeer                                 │
│ • VPeer instantiated with peerId                            │
│ • Session created (connection tracking)                     │
│ • Event subscription established (WebSocket/SSE)            │
│ • Peer registered in relay's peer list                      │
└────────────────────┬────────────────────────────────────────┘
                     ↓
        [VPeer is now "online" and visible]
        [Other peers can join groups with it]
                     ↓
┌─────────────────────────────────────────────────────────────┐
│ Client operates: join groups, send messages, initiate calls │
│ [All via HTTPS API, routed through VPeer]                  │
└────────────────────┬────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────────────────────┐
│ Client disconnects (or timeout after 30s no ping)           │
└────────────────────┬────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────────────────────┐
│ Grace Period (5s for reconnection)                          │
│ • VPeer still active, accepts same peerId                   │
│ • Other peers see as "last seen: 5s ago"                    │
└────────────────────┬────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────────────────────┐
│ Grace period expires                                        │
└────────────────────┬────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────────────────────┐
│ Cleanup:                                                    │
│ • Leave all groups                                          │
│ • Unsubscribe from realtime channels                        │
│ • Broadcast peer offline to other peers                     │
│ • Remove VPeer from manager                                 │
│ • Release session resources                                 │
└─────────────────────────────────────────────────────────────┘
```

### VPeer State Machine

```
     [UNINITIALIZED]
            ↓
         connect()
            ↓
      [ACTIVE] ←──────────────┐
       ↓    ↓                 │
   (ops) disconnect      reconnect()
       ↓    ↓                 │
    [IDLE] ──────────────────┘
       ↓
    [cleanup timeout]
       ↓
    [DELETED]
```

---

## API Design

### REST Endpoints

#### 1. Peer Connection & Status

```http
POST /api/peers/{peerId}/connect
Content-Type: application/json
Authorization: Bearer {token}

{
  "publicKey": "12D3KooX...",
  "metadata": {
    "name": "Alice's iPhone",
    "avatar": "data:image/...",
    "platform": "ios|android|web|desktop"
  }
}

Response (200 OK):
{
  "peerId": "mobile-alice",
  "status": "active",
  "sessionId": "sess_abc123",
  "relayAddr": "/dns4/goop2.com/tcp/443/wss/p2p/12D3KooXRelay...",
  "peers": [
    {
      "peerId": "local-bob",
      "lastSeen": "2026-03-02T10:15:00Z",
      "metadata": {...}
    }
  ]
}
```

```http
POST /api/peers/{peerId}/ping
Authorization: Bearer {token}

Response (204 No Content)
[Keeps connection alive, resets disconnect timeout]
```

```http
POST /api/peers/{peerId}/disconnect
Authorization: Bearer {token}

Response (200 OK):
{
  "message": "Disconnected gracefully"
}
[Immediate cleanup without grace period]
```

#### 2. Group Operations

```http
POST /api/peers/{peerId}/group/create
Content-Type: application/json
Authorization: Bearer {token}

{
  "groupId": "group-abc123",
  "name": "Project Backlog",
  "role": "host|member"
}

Response (201 Created):
{
  "groupId": "group-abc123",
  "members": [...],
  "createdAt": "2026-03-02T10:15:00Z"
}
```

```http
POST /api/peers/{peerId}/group/invite
Content-Type: application/json
Authorization: Bearer {token}

{
  "groupId": "group-abc123",
  "targetPeerId": "local-bob"
}

Response (200 OK):
{
  "message": "Invitation sent"
}
```

```http
POST /api/peers/{peerId}/group/join
Content-Type: application/json
Authorization: Bearer {token}

{
  "groupId": "group-abc123"
}

Response (200 OK):
{
  "groupId": "group-abc123",
  "members": [...]
}
[Subscription to group events opened via WebSocket/SSE]
```

```http
POST /api/peers/{peerId}/group/leave
Content-Type: application/json
Authorization: Bearer {token}

{
  "groupId": "group-abc123"
}

Response (200 OK)
```

#### 3. Realtime Channels

```http
POST /api/peers/{peerId}/realtime/subscribe
Content-Type: application/json
Authorization: Bearer {token}

{
  "channelId": "rt-call-session-xyz"
}

Response (200 OK):
{
  "channelId": "rt-call-session-xyz",
  "otherPeerId": "local-bob",
  "topic": "webrtc"
}
[Subscription opened]
```

```http
POST /api/peers/{peerId}/realtime/publish
Content-Type: application/json
Authorization: Bearer {token}

{
  "channelId": "rt-call-session-xyz",
  "message": "v=0\r\no=...",
  "type": "offer|answer|ice"
}

Response (200 OK):
{
  "message": "Published"
}
```

```http
POST /api/peers/{peerId}/realtime/unsubscribe
Content-Type: application/json
Authorization: Bearer {token}

{
  "channelId": "rt-call-session-xyz"
}

Response (200 OK)
```

#### 4. Chat/Messages

```http
POST /api/peers/{peerId}/message/send
Content-Type: application/json
Authorization: Bearer {token}

{
  "groupId": "group-abc123",
  "text": "Hello everyone!",
  "mentions": ["local-bob"]
}

Response (201 Created):
{
  "messageId": "msg_xyz123",
  "timestamp": "2026-03-02T10:15:00Z"
}
```

```http
GET /api/peers/{peerId}/message/history
?groupId=group-abc123
&limit=50
&before=2026-03-02T10:00:00Z
Authorization: Bearer {token}

Response (200 OK):
{
  "messages": [
    {
      "messageId": "msg_1",
      "peerId": "mobile-alice",
      "text": "Hello",
      "timestamp": "2026-03-02T10:00:00Z"
    }
  ]
}
```

### WebSocket Endpoint

```
WebSocket /ws/peers/{peerId}
Authorization: Bearer {token}

Client → Server:
{
  "type": "ping"
}

Server → Client (unsolicited events):
{
  "type": "peer_joined",
  "peerId": "local-bob",
  "metadata": {...},
  "timestamp": "2026-03-02T10:15:00Z"
}

{
  "type": "group_message",
  "groupId": "group-abc123",
  "peerId": "local-bob",
  "text": "Hey Alice!",
  "timestamp": "2026-03-02T10:15:01Z"
}

{
  "type": "realtime_message",
  "channelId": "rt-call-xyz",
  "peerId": "local-bob",
  "message": "v=0\r\no=...",
  "type": "offer"
}

{
  "type": "peer_left",
  "peerId": "local-bob",
  "reason": "disconnected|timeout|manual"
}

{
  "type": "session_timeout",
  "message": "Session will expire in 30 seconds. Send ping to extend."
}
```

### Server-Sent Events (Alternative to WebSocket)

```
GET /sse/peers/{peerId}
Authorization: Bearer {token}

Server → Client (continuous stream):
event: peer_joined
data: {"peerId": "local-bob", "metadata": {...}}

event: group_message
data: {"groupId": "group-abc123", "peerId": "local-bob", "text": "..."}

event: realtime_message
data: {"channelId": "rt-call-xyz", "peerId": "local-bob", "message": "..."}

event: peer_left
data: {"peerId": "local-bob"}
```

---

## Protocol Integration

### How Virtual Peers Integrate with Group Protocol

**Current Group Protocol Flow:**
1. Host creates group, gets groupId
2. Host invites member (local peer)
3. Member accepts → group has 2 peers
4. Host sends message → all peers receive

**With Virtual Peers:**

```
Step 1: Host (local-bob) invites remote (mobile-alice)
        ├─ local-bob: InvitePeer("mobile-alice", groupId)
        └─ Relay VPeer("mobile-alice") receives invite via relay function

Step 2: mobile-alice (HTTPS client) accepts via API
        ├─ POST /api/peers/mobile-alice/group/join
        └─ VPeer("mobile-alice").JoinGroup(groupId)

Step 3: VPeer updates group membership
        ├─ GroupManager now has [local-bob, mobile-alice]
        └─ Both peers are full members

Step 4: Host sends message
        ├─ local-bob: SendToGroup(groupId, "Hello")
        ├─ GroupManager broadcasts to all members
        ├─ local-bob receives locally
        └─ VPeer("mobile-alice") receives via internal call
            ├─ VPeer broadcasts to HTTPS client via WebSocket
            └─ Mobile app receives via /ws/peers/mobile-alice
```

### How Virtual Peers Integrate with Realtime Channels

**WebRTC Call Between Local & Remote Peer:**

```
Step 1: local-bob initiates call to mobile-alice
        ├─ RealtimeManager creates 2-peer group "rt-call-xyz"
        ├─ Invites both local-bob and mobile-alice
        └─ Topics: "offer", "answer", "ice"

Step 2: local-bob sends WebRTC offer
        ├─ local-bob: Publish(groupId, "v=0\r\no=...", type="offer")
        ├─ RealtimeManager broadcasts to group
        └─ VPeer("mobile-alice") receives via internal call
            ├─ Event routed to HTTPS client via WebSocket
            └─ Mobile app sends to WebRTC API

Step 3: mobile-alice sends answer
        ├─ Mobile sends: POST /api/peers/mobile-alice/realtime/publish
        │   body: { channelId, message: "v=0\r\na=...", type: "answer" }
        ├─ VPeer("mobile-alice").Publish() called
        ├─ RealtimeManager broadcasts
        └─ local-bob receives via P2P

Step 4: ICE candidates exchanged same way
```

### Addressing & Routing

**Virtual Peer Address:**
```
/dns4/goop2.com/tcp/443/wss/p2p/{relayPeerId}/vp/{virtualPeerId}
```

When a local peer wants to address a virtual peer:
1. Local peer queries relay: "Where is mobile-alice?"
2. Relay responds: "It's a virtual peer on me (/dns4/goop2.com/.../vp/mobile-alice)"
3. Local peer sends message to relay's libp2p address with virtual peer ID
4. Relay routes the message to the correct VPeer
5. VPeer broadcasts to HTTPS client via WebSocket

**Special Case - Direct Group Communication:**
- When both peers are in a group, the relay acts as group host/member
- The protocol already handles multi-peer messaging
- Virtual peer is just another member in the group's member list
- No special addressing needed within a group context

---

## Client Flows

### Mobile App Flow (iOS/Android)

```
┌──────────────────────────────────────────────────────┐
│ 1. App Startup                                       │
├──────────────────────────────────────────────────────┤
│ • Generate/load peerId (device UUID)                 │
│ • Load auth token from secure storage                │
│ • Get public key from local keychain                 │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 2. Connect to Relay                                  │
├──────────────────────────────────────────────────────┤
│ POST /api/peers/{peerId}/connect                     │
│ {                                                    │
│   "publicKey": "...",                                │
│   "metadata": { "name": "Alice iPhone", ... }        │
│ }                                                    │
│                                                      │
│ Response:                                            │
│ {                                                    │
│   "peerId": "mobile-alice",                          │
│   "sessionId": "...",                                │
│   "peers": [{...}, {...}]  # Available peers         │
│ }                                                    │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 3. Open WebSocket for Real-time Events              │
├──────────────────────────────────────────────────────┤
│ WebSocket /ws/peers/{peerId}                         │
│ (Keep open for the session)                          │
│                                                      │
│ Also: Send ping every 25s to keep-alive             │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 4. Discover & Browse Peers                          │
├──────────────────────────────────────────────────────┤
│ GET /api/peers/{peerId}/peers                        │
│ (Returns available peers, periodically or on demand) │
│                                                      │
│ App displays:                                        │
│ • Local peers (found by relay via mDNS)              │
│ • Other remote peers (other virtual peers)           │
│ • Last seen times, online status                     │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 5. User Selects Peer → Initiate Call                │
├──────────────────────────────────────────────────────┤
│ POST /api/peers/mobile-alice/realtime/subscribe      │
│ { "channelId": "rt-call-xyz" }                       │
│                                                      │
│ Relay creates realtime group with both peers        │
│ WebSocket notifies app: peer accepted call ready     │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 6. WebRTC Signaling                                 │
├──────────────────────────────────────────────────────┤
│ • Local RTCPeerConnection created                    │
│ • generateOffer()                                    │
│ • Send offer via: POST .../realtime/publish         │
│                                                      │
│ WebSocket receives answer from other peer           │
│ • setRemoteDescription(answer)                       │
│ • ICE candidates exchanged via WebSocket             │
│                                                      │
│ • Media connected → Audio/Video flow                 │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 7. Chat in Group                                    │
├──────────────────────────────────────────────────────┤
│ POST /api/peers/mobile-alice/group/join              │
│ { "groupId": "group-backlog" }                       │
│                                                      │
│ WebSocket receives:                                  │
│ - Group message from local peers                     │
│ - Peer joined/left events                            │
│                                                      │
│ Send message:                                        │
│ POST /api/peers/mobile-alice/message/send            │
│ { "groupId": "group-backlog", "text": "..." }        │
│                                                      │
│ Message delivered to all group members via P2P      │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 8. Disconnect / Go to Background                    │
├──────────────────────────────────────────────────────┤
│ • WebSocket connection drops (iOS background)        │
│ • Relay waits 30s (grace period)                     │
│ • Relay closes groups/realtime subs                  │
│ • App resumes → reconnect within grace period        │
│   (reuses same peerId + sessionId)                   │
│ OR timeout → full cleanup                           │
└──────────────────────────────────────────────────────┘
```

### Web Browser Flow (https://goop2.com)

```
┌──────────────────────────────────────────────────────┐
│ 1. Page Load                                         │
├──────────────────────────────────────────────────────┤
│ • Generate peerId (random UUID or from localStorage) │
│ • Load public/private key from localStorage          │
│ • Fetch /sdk/goop-*.js libraries                    │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 2. Connect to Relay (same as mobile)                │
├──────────────────────────────────────────────────────┤
│ POST /api/peers/{peerId}/connect                     │
│ WebSocket /ws/peers/{peerId}                         │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 3. Browse Available Peers                           │
├──────────────────────────────────────────────────────┤
│ Display:                                             │
│ • List of local peers (found via relay's mDNS)       │
│ • Other web/mobile users                             │
│ • Click to initiate connection                       │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 4. Call / Group Chat                                │
├──────────────────────────────────────────────────────┤
│ • WebRTC signaling (same as mobile)                  │
│ • Join groups / send messages (same as mobile)       │
│ • Video element plays audio/video stream             │
└──────────────────────────────────────────────────────┘
```

### Desktop Client Flow (mDNS + HTTPS Fallback)

```
┌──────────────────────────────────────────────────────┐
│ 1. Discover Local Peers (mDNS)                      │
├──────────────────────────────────────────────────────┤
│ • Scan local network via mDNS                        │
│ • Add to peer list                                   │
│ • Try direct P2P connection                          │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 2. Discover Remote Peers (HTTPS Bridge)             │
├──────────────────────────────────────────────────────┤
│ • Also connect to relay as virtual peer              │
│ • Same as mobile/web flow above                      │
│ • Can see both local AND remote peers                │
└──────────────────────────────────────────────────────┘
                        ↓
┌──────────────────────────────────────────────────────┐
│ 3. Smart Routing                                    │
├──────────────────────────────────────────────────────┤
│ When calling local peer:                             │
│ • Use direct P2P if possible (mDNS discovery)        │
│ • Fall back to relay if P2P fails                    │
│                                                      │
│ When calling remote peer:                            │
│ • Use relay (HTTPS virtual peer)                     │
│                                                      │
│ Result: Transparent, optimal routing                │
└──────────────────────────────────────────────────────┘
```

---

## Implementation Details

### 1. VirtualPeer Data Structure

```go
type VirtualPeer struct {
    // Identity
    PeerID       string                   // Unique ID (e.g., "mobile-alice")
    PublicKey    crypto.PublicKey         // For signing/verification
    SessionID    string                   // Current session tracking
    CreatedAt    time.Time
    LastActivity time.Time

    // Connection State
    ConnLock    sync.RWMutex
    IsActive    bool                      // Is the HTTPS connection alive?
    Conn        *websocket.Conn           // WebSocket connection (if active)
    EventChan   chan VPeerEvent           // Channel for events to send to client

    // Protocol State
    GroupMemberships map[string]*group.Group  // Groups this peer is in
    RealtimeSubs     map[string]*realtime.Channel

    // Metadata
    Metadata    PeerMetadata              // Name, avatar, platform, etc.

    // Grace Period
    DisconnectAt time.Time                // When to fully cleanup if not reconnected
}

type PeerMetadata struct {
    Name           string
    Avatar         string
    Platform       string  // "ios", "android", "web", "desktop"
    VideoDisabled  bool
    ActiveTemplate string
}

type VPeerEvent struct {
    Type      string                 // "peer_joined", "group_message", etc.
    Timestamp time.Time
    Payload   map[string]interface{} // Event-specific data
}
```

### 2. VirtualPeerManager

```go
type VirtualPeerManager struct {
    lock        sync.RWMutex
    peers       map[string]*VirtualPeer  // peerId → VPeer

    groups      *group.Manager            // Reference to main group manager
    realtime    *realtime.Manager         // Reference to main realtime manager

    // Grace period cleanup
    graceTicket *time.Ticker
    gracePeriod time.Duration  // 5 seconds

    // Keep-alive
    keepAliveTicket *time.Ticker
    keepAliveTimeout time.Duration  // 30 seconds of no ping
}

// Core operations
func (m *VirtualPeerManager) CreatePeer(peerId string, metadata PeerMetadata) (*VirtualPeer, error)
func (m *VirtualPeerManager) ActivateConnection(peerId string, conn *websocket.Conn) error
func (m *VirtualPeerManager) DisconnectPeer(peerId string, graceful bool) error
func (m *VirtualPeerManager) GetPeer(peerId string) *VirtualPeer
func (m *VirtualPeerManager) ListActivePeers() []*VirtualPeer
func (m *VirtualPeerManager) HandlePeerEvent(peerId string, event VPeerEvent) error
func (m *VirtualPeerManager) CleanupExpired() error
```

### 3. HTTPS Gateway Router

```go
type HTTPSGateway struct {
    mux       *http.ServeMux
    vpManager *VirtualPeerManager
    auth      *AuthProvider

    // Rate limiting, CORS, etc.
    rateLimiter RateLimiter
}

func (g *HTTPSGateway) RegisterRoutes() {
    // Peer lifecycle
    g.mux.HandleFunc("POST /api/peers/{peerId}/connect", g.handlePeerConnect)
    g.mux.HandleFunc("POST /api/peers/{peerId}/disconnect", g.handlePeerDisconnect)
    g.mux.HandleFunc("POST /api/peers/{peerId}/ping", g.handlePeerPing)

    // Groups
    g.mux.HandleFunc("POST /api/peers/{peerId}/group/create", g.handleGroupCreate)
    g.mux.HandleFunc("POST /api/peers/{peerId}/group/join", g.handleGroupJoin)
    g.mux.HandleFunc("POST /api/peers/{peerId}/group/leave", g.handleGroupLeave)
    g.mux.HandleFunc("POST /api/peers/{peerId}/group/invite", g.handleGroupInvite)

    // Realtime
    g.mux.HandleFunc("POST /api/peers/{peerId}/realtime/subscribe", g.handleRealtimeSubscribe)
    g.mux.HandleFunc("POST /api/peers/{peerId}/realtime/publish", g.handleRealtimePublish)
    g.mux.HandleFunc("POST /api/peers/{peerId}/realtime/unsubscribe", g.handleRealtimeUnsubscribe)

    // Messages
    g.mux.HandleFunc("POST /api/peers/{peerId}/message/send", g.handleMessageSend)
    g.mux.HandleFunc("GET /api/peers/{peerId}/message/history", g.handleMessageHistory)

    // WebSocket / SSE
    g.mux.HandleFunc("GET /ws/peers/{peerId}", g.handleWebSocket)
    g.mux.HandleFunc("GET /sse/peers/{peerId}", g.handleSSE)
}
```

### 4. WebSocket Event Loop

```go
func (g *HTTPSGateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
    peerId := r.PathValue("peerId")

    // Authenticate
    token := r.Header.Get("Authorization")
    if !g.auth.ValidateToken(peerId, token) {
        http.Error(w, "Unauthorized", 401)
        return
    }

    // Upgrade to WebSocket
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    // Get or create VPeer
    vpeer := g.vpManager.GetPeer(peerId)
    if vpeer == nil {
        vpeer, _ = g.vpManager.CreatePeer(peerId, PeerMetadata{})
    }

    // Activate connection
    g.vpManager.ActivateConnection(peerId, conn)
    defer g.vpManager.DisconnectPeer(peerId, true)

    // Event loop: client → server
    go func() {
        for {
            msg := ClientMessage{}
            err := conn.ReadJSON(&msg)
            if err != nil {
                vpeer.LastActivity = time.Now()
                return
            }

            vpeer.LastActivity = time.Now()

            switch msg.Type {
            case "ping":
                // Client indicates it's alive, reset timeout
            case "group_message_send":
                // Handle message
            // ... other message types
            }
        }
    }()

    // Event loop: server → client
    for event := range vpeer.EventChan {
        conn.WriteJSON(event)
    }
}
```

### 5. Group Integration Point

```go
// When VPeer joins a group:
func (vp *VirtualPeer) JoinGroup(ctx context.Context, groupId string) error {
    group, err := vp.groupManager.GetGroup(groupId)
    if err != nil {
        return err
    }

    // Add this virtual peer as a member
    group.AddMember(vp.PeerID, vp.PublicKey)

    // Subscribe to group events
    vp.GroupMemberships[groupId] = group

    // Listen for incoming messages from other group members
    go vp.listenGroupMessages(groupId)

    return nil
}

// When other peers send to the group:
func (g *Group) SendMessage(msg *Message) error {
    // Broadcast to all members
    for _, memberId := range g.Members {
        if memberId == msg.FromPeerId {
            continue  // Don't echo back
        }

        member := g.manager.GetPeer(memberId)

        if localPeer, ok := member.(*LocalPeer); ok {
            // Send to local peer via P2P
            localPeer.Receive(msg)
        } else if vPeer, ok := member.(*VirtualPeer); ok {
            // Send to virtual peer via event channel
            vPeer.EventChan <- VPeerEvent{
                Type: "group_message",
                Payload: map[string]interface{}{
                    "groupId": g.ID,
                    "message": msg,
                },
            }
        }
    }
    return nil
}
```

### 6. Keep-Alive & Grace Period Management

```go
func (m *VirtualPeerManager) StartMaintenanceLoop() {
    // Check for inactive peers every 5 seconds
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        m.lock.Lock()

        now := time.Now()
        for peerId, vpeer := range m.peers {
            // Check if in grace period
            if !vpeer.IsActive && vpeer.DisconnectAt.Before(now) {
                // Grace period expired, cleanup
                m.cleanupPeer(peerId)
                continue
            }

            // Check keep-alive timeout
            if vpeer.IsActive && now.Sub(vpeer.LastActivity) > m.keepAliveTimeout {
                // No ping for 30s, consider disconnected
                vpeer.IsActive = false
                vpeer.DisconnectAt = now.Add(m.gracePeriod)
            }
        }

        m.lock.Unlock()
    }
}

func (m *VirtualPeerManager) cleanupPeer(peerId string) {
    vpeer := m.peers[peerId]

    // Leave all groups
    for groupId := range vpeer.GroupMemberships {
        vpeer.groupManager.RemoveMember(groupId, peerId)
    }

    // Unsubscribe from all realtime channels
    for chanId := range vpeer.RealtimeSubs {
        vpeer.realtime.Unsubscribe(chanId, peerId)
    }

    // Broadcast to other peers
    m.broadcastPeerLeft(peerId)

    // Remove from map
    delete(m.peers, peerId)
}
```

---

## Data Structures

### Database Schema (SQLite Extensions)

```sql
-- Virtual peer sessions (for tracking online status)
CREATE TABLE IF NOT EXISTS virtual_peer_sessions (
    peer_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    public_key BLOB NOT NULL,
    metadata TEXT NOT NULL,  -- JSON
    connected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_activity TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    disconnect_scheduled_at TIMESTAMP
);

-- Virtual peer group memberships
CREATE TABLE IF NOT EXISTS virtual_peer_group_memberships (
    peer_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (peer_id, group_id),
    FOREIGN KEY (peer_id) REFERENCES virtual_peer_sessions(peer_id),
    FOREIGN KEY (group_id) REFERENCES groups(id)
);

-- Virtual peer realtime subscriptions
CREATE TABLE IF NOT EXISTS virtual_peer_realtime_subs (
    peer_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    subscribed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (peer_id, channel_id),
    FOREIGN KEY (peer_id) REFERENCES virtual_peer_sessions(peer_id)
);
```

---

## Examples

### Example 1: Mobile App Joins a Group Chat

**Setup**: Local peer "bob" has group "project-todos". Mobile peer "alice" connects to relay.

```
Timeline:
─────────

[Mobile App - Alice]
1. POST /api/peers/mobile-alice/connect
   → VirtualPeer("mobile-alice") created and activated

2. WebSocket /ws/peers/mobile-alice opened
   → Ready to receive events

[Local Peer - Bob (via goop2 running locally)]
3. Bob initiates: InvitePeer("mobile-alice", "project-todos")
   → Relay receives invite via group protocol

[Relay]
4. VirtualPeer("mobile-alice") receives invite event
   → Routes to HTTPS via WebSocket:
     {
       "type": "group_invited",
       "groupId": "project-todos",
       "fromPeerId": "bob"
     }

[Mobile App - Alice]
5. User taps "Accept"
   → POST /api/peers/mobile-alice/group/join
     { "groupId": "project-todos" }

[Relay]
6. VirtualPeer("mobile-alice").JoinGroup("project-todos")
   → groupManager.AddMember("mobile-alice")
   → groupManager broadcasts event

7. All peers receive notification
   → Bob's local instance: "alice joined"
   → Alice's virtual peer: "you joined"

[Bob's Client]
8. Displays: "alice (mobile) joined the group"

[Alice's Mobile App]
9. WebSocket receives:
   {
     "type": "group_joined",
     "groupId": "project-todos",
     "members": ["bob", "mobile-alice"]
   }

10. Alice types: "Hey!"
    → POST /api/peers/mobile-alice/message/send
       { "groupId": "project-todos", "text": "Hey!" }

[Relay]
11. VirtualPeer("mobile-alice").SendMessage() called
    → groupManager.SendMessage()
    → Broadcasts to all members

[Bob's Client]
12. Receives message via P2P:
    From: mobile-alice
    Text: "Hey!"

→ Both peers are now fully synchronized
```

### Example 2: WebRTC Call Between Local and Mobile Peer

**Setup**: Bob (local) and Alice (mobile) want to video call.

```
[Bob's Client (local)]
1. Initiates call to "mobile-alice"
   → ClientCall = {
       initiatorId: "bob",
       receiverId: "mobile-alice",
       channelId: "rt-call-xyz"
     }

[Relay]
2. CreateRealtimeChannel("rt-call-xyz")
   → Type: "call", topic: "webrtc"
   → Members: [bob, mobile-alice]

[Relay]
3. Routes to both peers:
   → Sends to Bob's P2P: "alice is ready for call"
   → Routes to Alice's virtual peer via WebSocket:
     {
       "type": "call_incoming",
       "callId": "rt-call-xyz",
       "fromPeerId": "bob"
     }

[Alice's Mobile App]
4. Receives call notification
   → Shows "Bob is calling..."
   → User taps "Accept"
   → POST /api/peers/mobile-alice/realtime/subscribe
     { "channelId": "rt-call-xyz" }

[Relay]
5. VirtualPeer("mobile-alice").SubscribeRealtime("rt-call-xyz")
   → Adds to realtime subscriptions
   → Both peers now ready to exchange SDP/ICE

[Bob's Client]
6. Creates RTCPeerConnection
   → generateOffer()
   → SDP offer = "v=0\r\no=alice ....."
   → POST to /api/peers/bob/realtime/publish
     {
       "channelId": "rt-call-xyz",
       "message": "v=0\r\no=alice...",
       "type": "offer"
     }

[Relay]
7. Bob's offer routed to realtime group
   → Broadcasts to "mobile-alice" member
   → VirtualPeer event channel:
     {
       "type": "realtime_message",
       "channelId": "rt-call-xyz",
       "peerId": "bob",
       "message": "v=0\r\no=alice...",
       "messageType": "offer"
     }

[Alice's Mobile App]
8. WebSocket receives offer
   → RTCPeerConnection.setRemoteDescription(offer)
   → generateAnswer()
   → SDP answer = "v=0\r\na=alice....."
   → POST /api/peers/mobile-alice/realtime/publish
     { "channelId": "rt-call-xyz", "message": "v=0\r\na=alice...", "type": "answer" }

[Relay]
9. Alice's answer routed back to Bob

[Bob's Client]
10. Receives answer via P2P
    → RTCPeerConnection.setRemoteDescription(answer)

[Both Peers]
11. ICE candidates exchanged similarly via realtime channel
    → Local candidate → POST /api/peers/.../realtime/publish
    → Remote receives via WebSocket

[Both Peers]
12. Once ICE completes, media flow connects
    → Bob's audio → WebRTC → Alice's mobile speaker
    → Alice's mobile mic → WebRTC → Bob's audio

→ Call established!
```

---

## Edge Cases & Reliability

### 1. Reconnection Within Grace Period

```
Timeline:
─────────

T=0s: Client WebSocket closes (network glitch)
      → VirtualPeer.IsActive = false
      → VirtualPeer.DisconnectAt = now + 5s
      → Groups/realtime still active (peer "at rest")

T=2s: Client reconnects with same peerId
      POST /api/peers/mobile-alice/connect
      → Relay checks: Is this peerId already in manager?
      → Yes! And grace period not expired
      → Activate connection on existing VirtualPeer
      → No re-join needed, groups still active
      → Groups don't see interruption

T=7s: If client HADN'T reconnected by now
      → Grace period expires
      → cleanupPeer() called
      → Leave all groups
      → Cleanup realtime subscriptions
      → Other peers see "mobile-alice left"
```

### 2. Handling Slow Network / Message Buffering

**Scenario**: Alice (mobile) sends message while network is unstable.

```
POST /api/peers/mobile-alice/message/send
{ "groupId": "project-todos", "text": "My message" }

[Relay Processing]
1. Receive request
2. Validate peerId exists and is active
3. Call VirtualPeer.SendMessage()
4. Call groupManager.SendMessage()
5. Get list of group members
6. For each member:
   - If local peer: send via P2P (standard flow)
   - If virtual peer: queue to EventChan
7. Return 201 Created immediately (don't wait for delivery)

[Event Queuing]
- VPeer has buffered EventChan (size: 1000)
- If WebSocket not connected: events queue in memory
- When client reconnects: all queued events delivered

[Potential Issue]
- If too many events queue and channel overflows
  → App must handle gracefully
  → Fetch message history via GET /api/peers/.../message/history
```

### 3. Simultaneous Connect/Disconnect

**Scenario**: Network flicker causes rapid disconnect/reconnect attempts.

```
[Relay Thread 1]
WebSocket close detected
→ DisconnectPeer(peerId, graceful: true)
→ Set IsActive = false, DisconnectAt = now + 5s

[Relay Thread 2 - Concurrent]
Client sends: POST /api/peers/{peerId}/connect
→ ActivateConnection()
→ Lock acquired, set IsActive = true, clear DisconnectAt

[Result]
- Both operations are serialized via lock
- VPeer ends up active
- No data loss
- Groups see brief "away" but not full disconnect
```

### 4. Peer Trying to Reach Deleted Virtual Peer

**Scenario**: Bob tries to invite alice, but alice's grace period just expired.

```
[Bob's Client]
POST /api/peers/bob/group/invite
{ "groupId": "project", "targetPeerId": "mobile-alice" }

[Relay]
1. Resolve targetPeerId "mobile-alice"
2. Check if exists in VPeerManager
3. Not found (already cleaned up)
4. Return 404 or "Peer offline"
5. Bob's client can:
   - Retry with exponential backoff
   - Show "Alice is offline"
   - Add to offline group (if supported)
```

### 5. Virtual Peer in Group, Connection Drops During Group Activity

**Scenario**: Alice is in a group video call, WebSocket drops suddenly.

```
[Before Disconnection]
- rt-call-xyz has members: [bob, mobile-alice]
- ICE candidates flowing

[WebSocket Drops]
- VirtualPeer.IsActive = false
- Grace period starts (5s)

[Grace Period (T=0-5s)]
- Alice can't send/receive (no WebSocket)
- But group still thinks she's a member
- Bob sends message: "Still here?"
  → Relay tries to queue event
  → EventChan buffers message
  → But no reader (WebSocket closed)
  → Channel fills/blocks

[Problem]
- EventChan blocking on send can deadlock the group operation
- Solution: Use non-blocking send with buffer overflow handling
  ```go
  select {
  case vp.EventChan <- event:
      // Sent successfully
  default:
      // Channel full, client probably disconnected
      // Log event, don't wait
  }
  ```

[Alice Reconnects (T=3s)]
- POST /api/peers/mobile-alice/connect
- Activate existing VirtualPeer
- Drain buffered events from queue
- Catch up on group state
- Re-send local ICE candidates

[Alice Doesn't Reconnect (T=5s)]
- Grace period expires
- cleanupPeer("mobile-alice") called
- Remove from rt-call-xyz
- Bob's client receives: "mobile-alice left the call"
```

### 6. Clock Skew / Server Time Issues

```
[Scenario]
Relay server time jumps forward 10 minutes (clock sync issue)

[Result for Active Peer]
- Alice's last_activity was 2s ago (relay time)
- Suddenly now - last_activity = 10m 2s > 30s timeout
- Peer marked as disconnected incorrectly
- Grace period starts even though WebSocket is open

[Solution]
- Use monotonic clock (time.Since) instead of wall-clock differences
- Track LastActivityTime as monotonic for timeout checks
- Only use wall-clock for DB/logs
```

---

## Security Considerations

### 1. Authentication & Authorization

**Per-Peer Token System:**
```go
type AuthToken struct {
    PeerID      string
    SessionID   string
    IssuedAt    time.Time
    ExpiresAt   time.Time
    Signature   []byte  // Signed by relay private key
}

// Client provides token with each request:
Authorization: Bearer eyJhbGciOiJFZDI1NTE5IiwidHlwIjoiSldUIn0...
```

**Token Validation:**
- Verify signature matches relay's key
- Check expiration
- Ensure peerId in token matches request peerId
- Optionally rotate tokens

**Session Isolation:**
- Each virtual peer is isolated
- Alice cannot operate as bob
- Tokens are per-peerId + per-device

### 2. Message Signing & Encryption

**In Transit (HTTPS/WSS):**
- All connections use TLS 1.3+
- No additional encryption needed for transit

**At Rest:**
- Messages in group storage: already encrypted by group protocol
- Virtual peer event queue: in-memory, not persisted

**Optional: End-to-End Signing:**
```
Mobile app before sending message:
1. Generate message body
2. Sign with device private key
3. Include signature in POST body
4. Relay forwards signature with message
5. Bob's client verifies signature
   → Confirms it really came from mobile-alice
```

### 3. Rate Limiting

**Per-Peer Limits:**
```
- POST /api/peers/{peerId}/* : 100 requests/minute
- WebSocket events: 50 messages/minute
- File uploads: 10MB/day
```

**Implementation:**
```go
type PeerRateLimit struct {
    APIReqs      *RateBucket  // 100/min
    WSEvents     *RateBucket  // 50/min
    FileBytes    int64        // Track daily
}

func (g *HTTPSGateway) checkRateLimit(peerId string) error {
    vpeer := g.vpManager.GetPeer(peerId)
    if !vpeer.rateLimit.APIReqs.Allow() {
        return fmt.Errorf("rate limit exceeded")
    }
    return nil
}
```

### 4. Denial of Service Prevention

**Connection Limits:**
- Max concurrent virtual peers: 10,000 per relay
- Max connections per IP: 100
- Grace period timeout: 5 seconds (free up resources)

**Message Size Limits:**
- Max message body: 1MB
- Max WebSocket frame: 64KB
- Max group size: 1000 peers (enforced by group protocol)

**Resource Cleanup:**
```go
// Every 5 seconds, cleanup expired peers
// Prevents accumulation of dead VPeers
func (m *VirtualPeerManager) CleanupExpired() {
    for peerId, vpeer := range m.peers {
        if time.Now().After(vpeer.DisconnectAt) {
            m.cleanupPeer(peerId)
        }
    }
}
```

### 5. Privacy & Data Leakage

**What the relay sees:**
- peerId (you chose it - can be anonymous)
- Metadata (name, avatar URL)
- Group membership list
- Message headers (timestamp, sender, recipient group)
- Message content (if not E2E encrypted)

**What relay DOESN'T see (with end-to-end crypto):**
- Message plaintext (if client uses E2E encryption)
- File contents (same)
- Call audio/video (encrypted by WebRTC)

**Recommendation:**
- Deploy relay on infrastructure you control
- Use TLS certificates with perfect forward secrecy
- Optionally add E2E encryption at application layer

### 6. Peering with Untrusted Relays

**Risk**: Relay operator is adversarial
- Can see your peerId
- Can see group memberships
- Can see message content

**Mitigations:**
- Use E2E encryption for sensitive content
- Use anonymized peerIds (random UUIDs, not email)
- Verify relay's TLS certificate (pinning)
- Run your own relay

---

## Future Enhancements

### 1. Geographically Distributed Relays

```
Current: Single relay at goop2.com

Future: Multiple relays
  ├─ goop2-us.com    (Virginia)
  ├─ goop2-eu.com    (Frankfurt)
  ├─ goop2-sg.com    (Singapore)

Client chooses based on latency
→ Relay.GetClosestRelay() returns nearest endpoint
```

### 2. Relay Federation

```
Relay A (us.goop2.com)          Relay B (eu.goop2.com)
  ├─ VPeer: mobile-alice          ├─ VPeer: web-bob
  └─ Local peer: desktop-charlie  └─ Local peer: laptop-dave

Goal: Alice (on A) calls Bob (on B)

Solution:
1. A and B establish federation link
2. A routes alice's offer to B
3. B delivers to bob's WebSocket
4. Works transparently
```

### 3. Offline Message Queuing

```
Current: Messages to offline peer are lost

Future: Queue + Delivery
  - Store message in DB (undelivered_messages table)
  - When peer reconnects: deliver queued messages
  - Configurable retention (24 hours, etc.)
```

### 4. Web/Mobile SDKs

```
JavaScript SDK (@goop/relay-client)
├─ PeerConnection class
├─ Group class
├─ RealtimeChannel class
├─ Automatic reconnection
├─ Offline queue handling
└─ TypeScript definitions

Swift SDK (GoopRelayClient)
├─ Same API as JS
├─ Native iOS integration
├─ Background task handling
└─ Keychain integration
```

### 5. Proxy Mode (Relay as Peer)

```
Current: Relay is invisible (only hosts virtual peers)

Future Option: Relay itself as a peer
  - relay.goop2.com appears as a "peer"
  - Can join groups
  - Can relay messages between disconnected groups
  - Useful for maintaining network cohesion
```

### 6. Peer-to-Peer Relay Upgrades

```
Current: Remote peers always go through relay

Future: Bootstrap via relay, then establish direct P2P
  1. Alice (mobile) and Bob (local) meet via relay
  2. Exchange WebRTC SDP
  3. If NAT traversal succeeds: establish direct connection
  4. Switch to direct P2P (lower latency)
  5. Fall back to relay if direct fails
```

---

## Microservice Deployment

### Important: HTTP vs HTTPS Clarification

Before diving into deployment, see `./HTTPS_vs_HTTP_INTERNAL.md` for detailed explanation:

- **External API** (Client → Bridge): `HTTPS://goop2.com/api/...` ✅ (TLS encryption required)
- **Internal API** (Bridge → Relay): `HTTP://localhost:8888/internal/api/...` ✅ (HTTP sufficient, no encryption needed)

**Why HTTP internally?** Trusted network (data center), firewall protection, no internet exposure. HTTPS would add CPU cost with zero security benefit. See the dedicated document for full threat model analysis.

---

### 1. Integration with goop2-services Repository

The bridge service follows the **same pattern** as existing goop2-services (credits, email, registration, templates):

#### File Structure

```
goop2-services/
├── cmd/
│   └── https-bridge-server/
│       └── main.go                 # Entry point
├── https/
│   ├── config.go                   # Config struct + LoadConfig()
│   ├── config.json                 # Default config template
│   ├── server.go                   # Server + RegisterRoutes()
│   ├── vpeer_manager.go            # VirtualPeerManager logic
│   ├── gateway.go                  # HTTPSGateway + route handlers
│   ├── relay_client.go             # HTTP client to main relay
│   ├── websocket.go                # WebSocket handlers
│   ├── events.go                   # Event structures + queuing
│   └── server_test.go              # Unit tests
├── Makefile                        # Add: build target for https-bridge
└── README.md                       # Service docs
```

#### cmd/https-bridge-server/main.go Pattern

```go
package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/petervdpas/goop2-services/https"
	"github.com/petervdpas/goop2-services/logbuf"
)

var appVersion = "dev"

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := https.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	srv := https.NewServer(cfg, appVersion)

	buf := logbuf.New(200)
	log.SetOutput(io.MultiWriter(os.Stderr, buf))

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	mux.HandleFunc("GET /api/logs", buf.Handler())
	mux.HandleFunc("GET /healthz", srv.handleHealthz)

	log.Printf("HTTPS Bridge service listening on %s", cfg.Addr)
	log.Printf("Relay endpoint: %s", cfg.RelayURL)
	log.Printf("Max virtual peers: %d", cfg.MaxVirtualPeers)

	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

#### https/config.go Pattern

```go
package https

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	// External API (clients connect here with TLS)
	Addr string `json:"addr"` // e.g., ":8804"

	// TLS Configuration (for external HTTPS clients)
	TLSCert string `json:"tls_cert"` // Path to cert.pem (or "" to disable local TLS)
	TLSKey  string `json:"tls_key"`   // Path to key.pem (or "" to disable local TLS)

	// Internal Relay Communication (HTTP, NOT HTTPS - see HTTPS_vs_HTTP_INTERNAL.md)
	RelayURL       string        `json:"relay_url"`       // http://localhost:8888 (INTERNAL: HTTP only)
	RelayAuthToken string        `json:"relay_auth_token"` // Bearer token for /internal/*
	PeerIDPrefix   string        `json:"peer_id_prefix"`  // "vp-" prefix for virtual peers

	// Virtual Peer Management
	MaxVirtualPeers      int           `json:"max_virtual_peers"`      // 10000
	GracePeriod          time.Duration `json:"grace_period"`           // 5s
	KeepAliveTimeout     time.Duration `json:"keep_alive_timeout"`     // 30s
	SessionCheckInterval time.Duration `json:"session_check_interval"` // 5s

	// Rate Limiting
	RateLimitPerMinute int `json:"rate_limit_per_minute"` // 100
	RateLimitWSPerMin  int `json:"rate_limit_ws_per_min"`  // 50

	// Logging
	LogLevel string `json:"log_level"` // "debug", "info", "warn", "error"
}

func DefaultConfig() Config {
	return Config{
		Addr:                 ":8804",
		TLSCert:              "/etc/goop2-services/https-bridge/cert.pem",
		TLSKey:               "/etc/goop2-services/https-bridge/key.pem",
		RelayURL:             "http://localhost:8888",
		PeerIDPrefix:         "vp-",
		MaxVirtualPeers:      10000,
		GracePeriod:          5 * time.Second,
		KeepAliveTimeout:     30 * time.Second,
		SessionCheckInterval: 5 * time.Second,
		RateLimitPerMinute:   100,
		RateLimitWSPerMin:    50,
		LogLevel:             "info",
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
```

#### https/server.go Pattern

```go
package https

import (
	"fmt"
	"log"
	"net/http"
)

const APIVersion = 1

type Server struct {
	cfg              Config
	version          string
	vpeerManager     *VirtualPeerManager
	relayClient      *RelayClient
	rateLimiters     map[string]*RateLimiter
}

func NewServer(cfg Config, version string) *Server {
	relayClient := NewRelayClient(cfg.RelayURL, cfg.RelayAuthToken)

	return &Server{
		cfg:              cfg,
		version:          version,
		vpeerManager:     NewVirtualPeerManager(cfg, relayClient),
		relayClient:      relayClient,
		rateLimiters:     make(map[string]*RateLimiter),
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Peer lifecycle
	mux.HandleFunc("POST /api/peers/{peerId}/connect", s.authMiddleware(s.handlePeerConnect))
	mux.HandleFunc("POST /api/peers/{peerId}/disconnect", s.authMiddleware(s.handlePeerDisconnect))
	mux.HandleFunc("POST /api/peers/{peerId}/ping", s.authMiddleware(s.handlePeerPing))

	// Groups
	mux.HandleFunc("POST /api/peers/{peerId}/group/create", s.authMiddleware(s.handleGroupCreate))
	mux.HandleFunc("POST /api/peers/{peerId}/group/join", s.authMiddleware(s.handleGroupJoin))
	mux.HandleFunc("POST /api/peers/{peerId}/group/leave", s.authMiddleware(s.handleGroupLeave))

	// Realtime
	mux.HandleFunc("POST /api/peers/{peerId}/realtime/subscribe", s.authMiddleware(s.handleRealtimeSubscribe))
	mux.HandleFunc("POST /api/peers/{peerId}/realtime/publish", s.authMiddleware(s.handleRealtimePublish))

	// Messages
	mux.HandleFunc("POST /api/peers/{peerId}/message/send", s.authMiddleware(s.handleMessageSend))
	mux.HandleFunc("GET /api/peers/{peerId}/message/history", s.authMiddleware(s.handleMessageHistory))

	// WebSocket
	mux.HandleFunc("GET /ws/peers/{peerId}", s.authMiddleware(s.handleWebSocket))

	// Service endpoints
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/topology", s.handleTopology)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
		"service": "goop2-https-bridge",
		"version": "%s",
		"apiVersion": %d,
		"activePeers": %d,
		"uptime": "%v"
	}`, s.version, APIVersion, s.vpeerManager.CountActivePeers(), "...")
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
		"services": [
			{"name": "goop2-https-bridge", "addr": "%s", "role": "bridge"}
		],
		"relayURL": "%s"
	}`, s.cfg.Addr, s.cfg.RelayURL)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// Check relay connectivity
	if err := s.relayClient.Ping(); err != nil {
		http.Error(w, "unhealthy: relay unreachable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("healthy"))
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		// TODO: Validate token (should match peerId in request)
		next(w, r)
	}
}
```

#### https/config.json

```json
{
  "addr": ":8804",
  "tls_cert": "/etc/goop2-services/https-bridge/cert.pem",
  "tls_key": "/etc/goop2-services/https-bridge/key.pem",
  "relay_url": "http://localhost:8888",
  "relay_auth_token": "secret-internal-token",
  "peer_id_prefix": "vp-",
  "max_virtual_peers": 10000,
  "grace_period": "5s",
  "keep_alive_timeout": "30s",
  "session_check_interval": "5s",
  "rate_limit_per_minute": 100,
  "rate_limit_ws_per_min": 50,
  "log_level": "info"
}
```

### 2. Makefile Target

```makefile
# Add to existing Makefile

HTTPS_BRIDGE_OUT = bin/goop2-service-https-bridge

build: $(HTTPS_BRIDGE_OUT) $(OTHER_SERVICES)

$(HTTPS_BRIDGE_OUT): cmd/https-bridge-server/main.go https/*.go
	mkdir -p bin
	go build -o $(HTTPS_BRIDGE_OUT) ./cmd/https-bridge-server

test:
	go test ./... -v

clean:
	rm -rf bin/
```

### 3. Systemd Integration

**File: /etc/systemd/system/goop2-https-bridge.service**

```ini
[Unit]
Description=Goop2 HTTPS Bridge Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=goop2
Group=goop2
ExecStart=/opt/goop2-services/https-bridge/goop2-service-https-bridge \
  -config=/etc/goop2-services/https-bridge/config.json
Restart=always
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Enable:**
```bash
sudo systemctl daemon-reload
sudo systemctl enable goop2-https-bridge
sudo systemctl start goop2-https-bridge
sudo systemctl status goop2-https-bridge
```

### 4. Systemd Changes to Main Relay

If the bridge is microservice-based, the main goop2 relay needs to:
1. Enable internal API endpoints (`/internal/api/*`)
2. Provide RelayClient implementation
3. Handle virtual peer routing

**File: systemd/goop2-relay.service** (if not already running)

```ini
[Unit]
Description=Goop2 P2P Relay
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/goop2/relay
Restart=always
RestartSec=5s
StandardOutput=journal
StandardError=journal
Environment="RELAY_INTERNAL_API_PORT=8888"
Environment="RELAY_INTERNAL_TOKEN=secret-internal-token"

[Install]
WantedBy=multi-user.target
```

### 5. Docker Compose (Optional)

For development/testing:

```yaml
version: "3.8"

services:
  goop2-relay:
    image: goop2-relay:latest
    ports:
      - "5001:5001"      # P2P
      - "8888:8888"      # Internal API
    environment:
      RELAY_INTERNAL_API_PORT: 8888
      RELAY_INTERNAL_TOKEN: secret-internal-token

  goop2-https-bridge:
    image: goop2-services:latest
    build:
      context: .
      dockerfile: Dockerfile.https-bridge
    ports:
      - "8804:8804"      # HTTPS (443 in prod)
    environment:
      RELAY_URL: http://goop2-relay:8888
      RELAY_AUTH_TOKEN: secret-internal-token
    depends_on:
      - goop2-relay
    volumes:
      - ./certs:/etc/certs:ro
```

### 6. Load Balancing (Production)

For scaling multiple bridge instances:

```
                    ┌─────────────────┐
                    │ Load Balancer   │
                    │ (NGINX/HAProxy) │
                    └────────┬────────┘
                             │ HTTPS:443
             ┌───────────────┼───────────────┐
             │               │               │
    ┌────────▼───┐  ┌────────▼───┐  ┌────────▼───┐
    │ Bridge #1  │  │ Bridge #2  │  │ Bridge #3  │
    │ :8804      │  │ :8805      │  │ :8806      │
    └────────┬───┘  └────────┬───┘  └────────┬───┘
             │               │               │
             └───────────────┼───────────────┘
                    HTTP:8888 (internal)
                             │
                    ┌────────▼────────┐
                    │ Main Relay      │
                    │ (P2P + groups)  │
                    └─────────────────┘
```

**NGINX Config:**
```nginx
upstream https_bridge {
    least_conn;
    server localhost:8804;
    server localhost:8805;
    server localhost:8806;
}

server {
    listen 443 ssl http2;
    server_name goop2.com;

    ssl_certificate /etc/letsencrypt/live/goop2.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/goop2.com/privkey.pem;

    location / {
        proxy_pass http://https_bridge;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 7. Deployment Checklist (Microservice)

- [ ] Create `https/` package in goop2-services
- [ ] Implement config.go, server.go, vpeer_manager.go
- [ ] Create cmd/https-bridge-server/main.go
- [ ] Add Makefile target for https-bridge
- [ ] Create systemd service file
- [ ] Generate TLS certificates
- [ ] Create default config.json
- [ ] Implement RelayClient (HTTP to main relay)
- [ ] Add /internal/* endpoints to main relay
- [ ] Test bridge ↔ relay communication
- [ ] Load test with 100+ concurrent virtual peers
- [ ] Set up Docker image build
- [ ] Document environment variables
- [ ] Create monitoring/alerting rules
- [ ] Deploy to staging
- [ ] Performance test across geography
- [ ] Deploy to production

---

## Deployment Checklist

### Shared (Both Embedded & Microservice)

- [ ] Design TLS certificate strategy (self-signed, Let's Encrypt, corporate CA)
- [ ] Implement VirtualPeerManager
- [ ] Implement HTTPSGateway with REST routes
- [ ] Add WebSocket endpoint
- [ ] Integrate with existing group protocol
- [ ] Integrate with existing realtime protocol
- [ ] Implement grace period cleanup loop
- [ ] Implement keep-alive timeout mechanism
- [ ] Add rate limiting (per-peer + per-IP)
- [ ] Add comprehensive logging
- [ ] Implement health check endpoints (/healthz)
- [ ] Add monitoring hooks (Prometheus metrics)
- [ ] Test mobile app connection
- [ ] Test web browser connection
- [ ] Test group chat flow
- [ ] Test WebRTC call flow (signaling)
- [ ] Load test (100+ concurrent virtual peers)
- [ ] Security review (auth, signing, rate limits)
- [ ] Penetration test HTTPS endpoints
- [ ] Document client SDKs (JS, Swift, Kotlin)

### Embedded in Main Relay

- [ ] Add HTTPS handler to existing relay
- [ ] Enable TLS on relay (https://goop2.com)
- [ ] Generate relay peer ID and keys
- [ ] Single systemd service with both P2P + HTTPS
- [ ] Performance test (P2P + HTTPS load)
- [ ] Deploy as main relay service

### Microservice (goop2-services)

- [ ] Add `https/` package to goop2-services
- [ ] Create cmd/https-bridge-server/main.go
- [ ] Create config.json template
- [ ] Add Makefile target for https-bridge build
- [ ] Create systemd service file (goop2-https-bridge.service)
- [ ] Implement RelayClient for internal HTTP API
- [ ] Add /internal/api/* endpoints to main relay
- [ ] Create config.json for bridge instance(s)
- [ ] Test bridge ↔ relay communication over localhost:8888
- [ ] Set up Docker image build
- [ ] Create NGINX/HAProxy load balancer config (multi-instance)
- [ ] Document environment variables + config fields
- [ ] Create monitoring/alerting rules
- [ ] Performance test (latency between bridge & relay)
- [ ] Test graceful shutdown + reconnection
- [ ] Deploy to staging
- [ ] Deploy to production

---

## Appendix: Configuration

### Option A: Embedded Bridge in Main Relay

**Example relay-config.yaml:**
```yaml
server:
  port: 5001
  peerId: "relay-001"

https:
  port: 443
  tlsCert: /path/to/cert.pem
  tlsKey: /path/to/key.pem

virtualPeers:
  maxConcurrent: 10000
  gracePeriodSeconds: 5
  keepAliveTimeoutSeconds: 30

rateLimit:
  apiRequestsPerMinute: 100
  wsMessagesPerMinute: 50

logging:
  level: info
  format: json

mdns:
  enabled: true
  announceInterval: 30s

tls:
  minVersion: "1.3"
  preferredCiphers:
    - TLS_AES_256_GCM_SHA384
    - TLS_CHACHA20_POLY1305_SHA256
```

### Option B: Microservice Bridge (goop2-services)

**Example /etc/goop2-services/https-bridge/config.json:**
```json
{
  "addr": ":8804",
  "tls_cert": "/etc/goop2-services/https-bridge/cert.pem",
  "tls_key": "/etc/goop2-services/https-bridge/key.pem",
  "relay_url": "http://localhost:8888",
  "relay_auth_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "peer_id_prefix": "vp-",
  "max_virtual_peers": 10000,
  "grace_period": "5s",
  "keep_alive_timeout": "30s",
  "session_check_interval": "5s",
  "rate_limit_per_minute": 100,
  "rate_limit_ws_per_min": 50,
  "log_level": "info"
}
```

**Main relay config change:**
```yaml
server:
  port: 5001
  peerId: "relay-001"

# Internal API for bridge communication
internalAPI:
  port: 8888
  authToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  allowedOrigins:
    - "http://localhost:8804"  # Bridge instance 1
    - "http://localhost:8805"  # Bridge instance 2 (load balancer)

mdns:
  enabled: true
  announceInterval: 30s
```

### Multi-Instance Load Balancing (Production)

**NGINX upstream config:**
```nginx
upstream goop2_https_bridge {
    least_conn;  # Balance by active connections

    server localhost:8804 max_fails=3 fail_timeout=10s;
    server localhost:8805 max_fails=3 fail_timeout=10s;
    server localhost:8806 max_fails=3 fail_timeout=10s;

    # Optional: stick sessions by peerId for affinity
    # hash $http_x_peer_id consistent;
}

server {
    listen 443 ssl http2;
    server_name goop2.com;

    ssl_certificate /etc/letsencrypt/live/goop2.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/goop2.com/privkey.pem;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    ssl_protocols TLSv1.3;

    # Proxy to bridge pool
    location / {
        proxy_pass http://goop2_https_bridge;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 3600s;  # Long timeout for WebSocket
    }
}
```

**Systemd environment for multi-instance:**
```bash
# /etc/goop2-services/https-bridge-{1,2,3}/config.json
# Each instance has unique addr (:8804, :8805, :8806)
# All point to same relay URL (http://localhost:8888)
```

---

**End of Document**
