# Video/Voice Chat for Goop2

## Overview

Add optional video and voice chat capabilities for:
- Direct peer-to-peer conversations
- Turn-based games (chess, tic-tac-toe, etc.)

Both parties must explicitly accept a call before media streams are established.

## Architecture

```
┌─────────────┐                              ┌─────────────┐
│   Peer A    │                              │   Peer B    │
│             │                              │             │
│  ┌───────┐  │    Signaling via Goop P2P    │  ┌───────┐  │
│  │WebRTC │◄─┼──────────────────────────────┼─►│WebRTC │  │
│  │ API   │  │  (SDP offers, ICE candidates)│  │ API   │  │
│  └───┬───┘  │                              │  └───┬───┘  │
│      │      │                              │      │      │
│      ▼      │   Direct Media Stream (UDP)  │      ▼      │
│  ┌───────┐  │◄════════════════════════════►│  ┌───────┐  │
│  │Camera │  │   (encrypted, peer-to-peer)  │  │Camera │  │
│  │ Mic   │  │                              │  │ Mic   │  │
│  └───────┘  │                              │  └───────┘  │
└─────────────┘                              └─────────────┘
```

## Signaling Flow

WebRTC requires a signaling channel to exchange connection metadata. Goop's existing P2P messaging handles this perfectly.

### Call Initiation

```
1. Peer A clicks "Start Video Call"
2. A sends call_request message to B via Goop
3. B sees incoming call notification
4. B clicks "Accept" or "Decline"
5. If accepted, WebRTC negotiation begins
```

### WebRTC Negotiation (via Goop messages)

```
A                           Goop P2P                         B
│                              │                              │
│── call_request ─────────────►│─────────────────────────────►│
│                              │                              │
│◄─────────────────────────────│◄──────── call_accepted ──────│
│                              │                              │
│── sdp_offer ────────────────►│─────────────────────────────►│
│                              │                              │
│◄─────────────────────────────│◄──────── sdp_answer ─────────│
│                              │                              │
│◄─────────────────────────────│◄──────── ice_candidate ──────│
│── ice_candidate ────────────►│─────────────────────────────►│
│                              │                              │
│◄═══════════════ Direct WebRTC Connection ══════════════════►│
```

## Message Types

New message types for `goop-data.js`:

```javascript
// Call request (A → B)
{
  type: "call_request",
  call_id: "uuid",
  media: ["video", "audio"],  // or just ["audio"]
  context: "chess_game_123"   // optional: game context
}

// Call response (B → A)
{
  type: "call_response",
  call_id: "uuid",
  accepted: true
}

// SDP Offer (A → B)
{
  type: "sdp_offer",
  call_id: "uuid",
  sdp: "v=0\r\no=- ..."  // SDP string
}

// SDP Answer (B → A)
{
  type: "sdp_answer",
  call_id: "uuid",
  sdp: "v=0\r\no=- ..."
}

// ICE Candidate (bidirectional)
{
  type: "ice_candidate",
  call_id: "uuid",
  candidate: {
    candidate: "candidate:...",
    sdpMid: "0",
    sdpMLineIndex: 0
  }
}

// Call end (either party)
{
  type: "call_end",
  call_id: "uuid",
  reason: "user_hangup"  // or "declined", "timeout", "error"
}
```

## Client API

New `Goop.call` module:

```javascript
// goop-call.js

Goop.call = {
  // Start a call to a peer
  async start(peerId, options = {}) {
    // options: { video: true, audio: true, context: null }
    // Returns: Call object
  },

  // Accept incoming call
  async accept(callId) {
    // Returns: Call object with media streams
  },

  // Decline incoming call
  decline(callId, reason = "declined"),

  // End active call
  end(callId),

  // Event handlers
  onIncoming: null,  // (callRequest) => {}
  onEnded: null,     // (callId, reason) => {}
};

// Call object
{
  id: "uuid",
  peerId: "12D3Koo...",
  peerLabel: "Alice",
  localStream: MediaStream,
  remoteStream: MediaStream,
  state: "connecting" | "connected" | "ended",

  // Methods
  mute(audio = true, video = false),
  unmute(audio = true, video = false),
  end(),

  // Events
  onStateChange: null,
  onRemoteStream: null,
}
```

## UI Components

### Incoming Call Modal

```
┌─────────────────────────────────┐
│                                 │
│    📹 Incoming Video Call       │
│                                 │
│    Alice wants to video chat    │
│                                 │
│   ┌─────────┐    ┌─────────┐   │
│   │ Decline │    │ Accept  │   │
│   └─────────┘    └─────────┘   │
│                                 │
└─────────────────────────────────┘
```

### Active Call Overlay

```
┌─────────────────────────────────────┐
│ ┌─────────────────────────────────┐ │
│ │                                 │ │
│ │      Remote Video (large)       │ │
│ │                                 │ │
│ │                     ┌─────────┐ │ │
│ │                     │ Local   │ │ │
│ │                     │ (small) │ │ │
│ │                     └─────────┘ │ │
│ └─────────────────────────────────┘ │
│                                     │
│    🎤 Mute   📹 Video   🔴 End     │
└─────────────────────────────────────┘
```

### In-Game Integration (Chess example)

```
┌───────────────────────────────────────────┐
│  You: White — Opponent's turn             │
├───────────────────────────────────────────┤
│  ┌─────────────────────┐  ┌────────────┐  │
│  │                     │  │            │  │
│  │    Chess Board      │  │  Opponent  │  │
│  │                     │  │   Video    │  │
│  │    ♜ ♞ ♝ ♛ ♚ ♝ ♞ ♜ │  │            │  │
│  │    ♟ ♟ ♟ ♟ ♟ ♟ ♟ ♟ │  ├────────────┤  │
│  │                     │  │    You     │  │
│  │    ♙ ♙ ♙ ♙ ♙ ♙ ♙ ♙ │  │  (muted)   │  │
│  │    ♖ ♘ ♗ ♕ ♔ ♗ ♘ ♖ │  │            │  │
│  │                     │  └────────────┘  │
│  └─────────────────────┘                  │
│                                           │
│  [Resign]  [🎤 Unmute]  [📹 Off]  [End]  │
└───────────────────────────────────────────┘
```

## NAT Traversal

### STUN Servers

Use public STUN servers for most connections:

```javascript
const iceServers = [
  { urls: "stun:stun.l.google.com:19302" },
  { urls: "stun:stun1.l.google.com:19302" },
  { urls: "stun:stun.cloudflare.com:3478" }
];
```

### TURN Fallback (Optional)

For restrictive networks, TURN relay may be needed:

```javascript
// Could be provided by rendezvous server or self-hosted
{
  urls: "turn:turn.example.com:3478",
  username: "user",
  credential: "pass"
}
```

**Options for TURN:**
1. Self-hosted coturn server
2. Cloudflare Calls (free tier available)
3. Twilio TURN (paid)
4. Make it optional - most connections work with STUN only

## Security & Privacy

### Consent Model

- **Explicit opt-in**: Both parties must accept before any media access
- **No auto-answer**: Never automatically accept calls
- **Browser permissions**: Standard browser camera/mic permission prompts
- **Visual indicators**: Clear UI showing when camera/mic are active

### Encryption

- WebRTC uses DTLS-SRTP encryption by default
- Media streams are encrypted end-to-end
- Signaling through Goop's existing encryption

### Privacy Controls

```javascript
// User preferences (stored locally)
{
  allowVideoCalls: true,
  allowAudioCalls: true,
  autoMuteOnJoin: false,
  cameraOffByDefault: false,
  blockedPeers: ["12D3Koo..."]
}
```

## Implementation Phases

### Phase 1: Core Infrastructure
- [ ] Add message types for call signaling
- [ ] Implement `Goop.call` JavaScript module
- [ ] Basic WebRTC connection establishment
- [ ] Audio-only calls working

### Phase 2: Video & UI
- [ ] Add video support
- [ ] Incoming call modal component
- [ ] Active call overlay component
- [ ] Mute/unmute, camera on/off controls

### Phase 3: Game Integration
- [ ] Add "Start Call" button to chess template
- [ ] Side-by-side layout for video + game
- [ ] Auto-cleanup when game ends

### Phase 4: Polish
- [ ] Connection quality indicator
- [ ] Screen share support (optional)
- [ ] Picture-in-picture mode
- [ ] Mobile touch controls

## File Structure

```
internal/assets/js/
  goop-call.js          # WebRTC + call management

internal/rendezvous/storetemplates/
  _shared/
    components/
      call-modal.js     # Incoming call UI
      call-overlay.js   # Active call UI
      call-controls.js  # Mute, end, etc.
    css/
      call.css          # Call UI styles

  chess/
    js/app.js           # Add call integration
```

## Dependencies

**None required** - uses native browser APIs:
- `RTCPeerConnection` - WebRTC
- `navigator.mediaDevices.getUserMedia()` - Camera/mic access
- `MediaStream` - Audio/video streams

## Open Questions

1. **TURN servers**: Self-host or use a service? Most P2P connections work without TURN.

2. **Group calls**: Mesh topology (2-4 people) or defer to future SFU implementation?

3. **Recording**: Allow recording calls? Privacy implications.

4. **Mobile**: Test on mobile browsers - may need UI adjustments.

## References

- [WebRTC API](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API)
- [Perfect Negotiation Pattern](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API/Perfect_negotiation)
- [STUN/TURN Overview](https://webrtc.org/getting-started/turn-server)
