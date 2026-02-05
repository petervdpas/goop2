# Real-Time Transport Layer for Goop2

## Problem Statement

Store templates currently rely on **polling** for real-time updates (e.g., chess uses 2-second intervals). This is:
- Inefficient (constant HTTP requests)
- High latency (up to 2 seconds)
- Insufficient for video chat signaling (needs <100ms)

Meanwhile, goop2 already has real-time capabilities via the **group protocol** - they're just not exposed to templates.

## Current Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Frontend                              │
├─────────────────────────────────────────────────────────────┤
│  goop-data.js     │  goop-peers.js   │  goop-identity.js   │
│  (HTTP polling)   │  (SSE events)    │  (local identity)   │
├─────────────────────────────────────────────────────────────┤
│                      HTTP Server                             │
├─────────────────────────────────────────────────────────────┤
│                        Go Backend                            │
├───────────────┬─────────────────────┬───────────────────────┤
│  Data Manager │   Group Manager     │    Chat Manager       │
│  (SQLite)     │   (real-time!)      │    (one-shot)         │
├───────────────┴─────────────────────┴───────────────────────┤
│                     libp2p Node                              │
│         TCP/QUIC • Yamux • Gossip PubSub                    │
└─────────────────────────────────────────────────────────────┘
```

### What Exists (But Isn't Exposed)

| Component | Protocol | Capabilities |
|-----------|----------|--------------|
| **Group Manager** | `/goop/group/1.0.0` | Persistent bidirectional streams, JSON messages, member events |
| **Chat Manager** | `/goop/chat/1.0.0` | Point-to-point messages (one-shot) |
| **Gossip PubSub** | `goop.presence.v1` | Broadcast to all peers |

The **Group Manager** is exactly what we need - it maintains persistent connections with real-time message delivery.

## Proposed Solution

### New Module: `goop-realtime.js`

Expose group protocol capabilities to templates via a new JavaScript API.

```
┌─────────────────────────────────────────────────────────────┐
│                        Frontend                              │
├─────────────────────────────────────────────────────────────┤
│  goop-data.js  │  goop-peers.js  │  goop-realtime.js  │ NEW │
│  (HTTP)        │  (SSE)          │  (WebSocket/SSE)    │     │
├─────────────────────────────────────────────────────────────┤
│                      HTTP Server                             │
│                 + WebSocket endpoint                         │
├─────────────────────────────────────────────────────────────┤
│                        Go Backend                            │
├───────────────┬─────────────────────┬───────────────────────┤
│  Data Manager │   Group Manager     │    Realtime Bridge    │
│               │        ▲            │          ▲            │
│               │        └────────────┼──────────┘            │
└───────────────┴─────────────────────┴───────────────────────┘
```

## API Design

### `goop-realtime.js`

```javascript
// Connect to a peer for real-time messaging
const channel = await Goop.realtime.connect(peerId, {
  // Optional: associate with a context (game, call, etc.)
  context: "chess_game_123"
});

// Send a message
channel.send({
  type: "move",
  from: "e2",
  to: "e4"
});

// Receive messages
channel.onMessage = function(msg) {
  console.log("Received:", msg);
};

// Connection state
channel.onStateChange = function(state) {
  // "connecting" | "connected" | "disconnected"
};

// Clean up
channel.close();
```

### Full API

```javascript
Goop.realtime = {
  // =========================================================================
  // Peer-to-Peer Channels
  // =========================================================================

  /**
   * Open a real-time channel to a specific peer.
   * Uses group protocol under the hood (2-peer private group).
   *
   * @param {string} peerId - Target peer ID
   * @param {object} options - { context?: string, timeout?: number }
   * @returns {Promise<Channel>}
   */
  async connect(peerId, options = {}) {},

  /**
   * Accept an incoming channel request.
   * Called when onIncoming fires.
   *
   * @param {string} channelId
   * @returns {Promise<Channel>}
   */
  async accept(channelId) {},

  /**
   * Decline an incoming channel request.
   *
   * @param {string} channelId
   * @param {string} reason - Optional reason
   */
  decline(channelId, reason = "declined") {},

  // =========================================================================
  // Events
  // =========================================================================

  /**
   * Called when a peer wants to open a channel.
   * @type {function({ channelId, peerId, peerLabel, context })}
   */
  onIncoming: null,

  // =========================================================================
  // Multi-Peer Rooms (future)
  // =========================================================================

  /**
   * Create or join a room for multi-peer real-time communication.
   * Built on top of group protocol.
   *
   * @param {string} roomId - Unique room identifier
   * @param {object} options - { maxPeers?: number }
   * @returns {Promise<Room>}
   */
  async joinRoom(roomId, options = {}) {},
};

// =========================================================================
// Channel Object (returned by connect/accept)
// =========================================================================
{
  id: "channel-uuid",
  peerId: "12D3Koo...",
  peerLabel: "Alice",
  context: "chess_game_123",
  state: "connecting" | "connected" | "disconnected",

  /**
   * Send a message to the peer.
   * @param {object} data - JSON-serializable object
   */
  send(data) {},

  /**
   * Close the channel.
   */
  close() {},

  // Events
  onMessage: null,      // (data) => {}
  onStateChange: null,  // (state) => {}
}

// =========================================================================
// Room Object (future, for multi-peer)
// =========================================================================
{
  id: "room-uuid",
  members: [{ peerId, peerLabel }],
  state: "joining" | "joined" | "left",

  send(data) {},                    // Broadcast to all
  sendTo(peerId, data) {},          // Send to specific peer
  leave() {},

  onMessage: null,       // (peerId, data) => {}
  onMemberJoin: null,    // (peer) => {}
  onMemberLeave: null,   // (peer) => {}
  onStateChange: null,   // (state) => {}
}
```

## Backend Changes

### New HTTP Endpoints

```
GET  /api/realtime/connect    → Upgrade to WebSocket
POST /api/realtime/request    → Request channel with peer
POST /api/realtime/accept     → Accept incoming channel
POST /api/realtime/decline    → Decline incoming channel
POST /api/realtime/send       → Send message on channel
POST /api/realtime/close      → Close channel
```

### WebSocket Protocol

Browser connects via WebSocket for real-time message delivery:

```javascript
// Client → Server
{ "type": "subscribe", "channels": ["channel-123"] }
{ "type": "send", "channel": "channel-123", "data": {...} }
{ "type": "close", "channel": "channel-123" }

// Server → Client
{ "type": "message", "channel": "channel-123", "from": "12D3Koo...", "data": {...} }
{ "type": "state", "channel": "channel-123", "state": "connected" }
{ "type": "incoming", "channel": "channel-456", "peerId": "12D3Koo...", "context": "..." }
```

### Realtime Bridge (Go)

New component that bridges WebSocket connections to Group Manager:

```go
// internal/realtime/bridge.go

type Bridge struct {
    groupMgr  *group.Manager
    channels  map[string]*Channel  // channelId → Channel
    wsConns   map[*websocket.Conn]struct{}
}

// Channel wraps a 2-peer group for point-to-point realtime
type Channel struct {
    ID        string
    LocalPeer string
    RemotePeer string
    Context   string
    Group     *group.Group  // Underlying group protocol connection
}

func (b *Bridge) HandleWebSocket(w http.ResponseWriter, r *http.Request)
func (b *Bridge) RequestChannel(peerId, context string) (*Channel, error)
func (b *Bridge) AcceptChannel(channelId string) (*Channel, error)
func (b *Bridge) SendMessage(channelId string, data []byte) error
func (b *Bridge) CloseChannel(channelId string) error
```

## Use Cases

### 1. Chess with Real-Time Moves

Replace 2-second polling with instant updates:

```javascript
// chess/js/app.js

let channel = null;

async function startPvPGame(opponentId) {
  // Open real-time channel
  channel = await Goop.realtime.connect(opponentId, {
    context: "chess_" + gameId
  });

  channel.onMessage = function(msg) {
    if (msg.type === "move") {
      applyMove(msg.from, msg.to, msg.promotion);
      renderGame();
    } else if (msg.type === "resign") {
      handleResign();
    }
  };

  channel.onStateChange = function(state) {
    if (state === "disconnected") {
      showMessage("Opponent disconnected");
    }
  };
}

async function makeMove(from, to, promotion) {
  // Send move to opponent instantly
  channel.send({
    type: "move",
    from: from,
    to: to,
    promotion: promotion
  });

  // Also save to database for persistence
  await db.call("move", { game_id: gameId, from, to, promotion });
}
```

### 2. Video Chat Signaling

Use channels for WebRTC signaling:

```javascript
// Start video call
async function startCall(peerId) {
  const channel = await Goop.realtime.connect(peerId, {
    context: "video_call"
  });

  // Create WebRTC peer connection
  const pc = new RTCPeerConnection({ iceServers });

  // Send ICE candidates via channel
  pc.onicecandidate = (e) => {
    if (e.candidate) {
      channel.send({
        type: "ice_candidate",
        candidate: e.candidate
      });
    }
  };

  // Receive signaling messages
  channel.onMessage = async (msg) => {
    if (msg.type === "sdp_offer") {
      await pc.setRemoteDescription(msg.sdp);
      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      channel.send({ type: "sdp_answer", sdp: answer });
    } else if (msg.type === "sdp_answer") {
      await pc.setRemoteDescription(msg.sdp);
    } else if (msg.type === "ice_candidate") {
      await pc.addIceCandidate(msg.candidate);
    }
  };

  // Create and send offer
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  channel.send({ type: "sdp_offer", sdp: offer });
}
```

### 3. Real-Time Multiplayer Games

For Space Invaders co-op or versus mode:

```javascript
// Join game room
const room = await Goop.realtime.joinRoom("arcade_" + gameId, {
  maxPeers: 4
});

room.onMemberJoin = (peer) => {
  addPlayer(peer.peerId, peer.peerLabel);
};

room.onMemberLeave = (peer) => {
  removePlayer(peer.peerId);
};

room.onMessage = (peerId, msg) => {
  if (msg.type === "position") {
    updatePlayerPosition(peerId, msg.x, msg.y);
  } else if (msg.type === "shoot") {
    createBullet(peerId, msg.x, msg.y, msg.direction);
  }
};

// Game loop broadcasts position
function gameLoop() {
  room.send({
    type: "position",
    x: player.x,
    y: player.y
  });
}
```

## Implementation Phases

### Phase 1: Core Infrastructure
- [ ] Add WebSocket endpoint to HTTP server
- [ ] Create `realtime.Bridge` that wraps Group Manager
- [ ] Implement `goop-realtime.js` with `connect()`, `send()`, `close()`
- [ ] Basic channel establishment between two peers

### Phase 2: Chess Integration
- [ ] Replace chess polling with real-time channels
- [ ] Test latency improvements
- [ ] Handle reconnection gracefully

### Phase 3: Video Chat
- [ ] Integrate with `goop-call.js` for WebRTC signaling
- [ ] Test SDP/ICE exchange latency
- [ ] Add call-specific UI

### Phase 4: Multi-Peer Rooms
- [ ] Implement `joinRoom()` API
- [ ] Test with 3-4 peers
- [ ] Add to Space Invaders for multiplayer

## Performance Expectations

| Metric | Polling (Current) | Real-Time (Proposed) |
|--------|-------------------|----------------------|
| Message latency | 2000ms avg | 50-200ms |
| Messages/second | 0.5 | 10+ |
| HTTP requests | Constant | Initial + WS |
| Battery/CPU | Higher | Lower |

## Security Considerations

1. **Channel Authorization**: Only allow channels between peers that have discovered each other
2. **Rate Limiting**: Limit messages per second per channel
3. **Message Size**: Cap message size (e.g., 64KB)
4. **Context Validation**: Ensure context (game ID) is valid before allowing channel

## File Structure

```
internal/
  realtime/
    bridge.go       # WebSocket ↔ Group Manager bridge
    channel.go      # Point-to-point channel
    room.go         # Multi-peer room (Phase 4)
    handlers.go     # HTTP/WebSocket handlers

internal/ui/assets/js/
  goop-realtime.js  # Frontend API

internal/rendezvous/storetemplates/
  chess/
    js/app.js       # Updated to use realtime
```

## Dependencies

**None new** - uses existing:
- `gorilla/websocket` (already in go.mod for other features)
- `group.Manager` (existing)
- Standard browser WebSocket API

## Open Questions

1. **Fallback**: If WebSocket fails, fall back to long-polling or SSE?
2. **Persistence**: Should channel messages be logged for debugging?
3. **Presence**: Show "typing" or "thinking" indicators in games?
4. **Compression**: Compress large messages (SDP can be 2-3KB)?

## References

- [Group Manager Implementation](../../internal/group/manager.go)
- [Video Chat Design](./video-chat.md)
- [WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
