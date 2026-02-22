# Pion WebRTC — Native Call Stack

## Why

On Linux, Wails embeds WebKitGTK as the webview. WebKitGTK does not support
`navigator.mediaDevices.getUserMedia()` reliably, breaking video/audio calls.

Solution: move camera/mic capture and the WebRTC P2P stack entirely into Go using
Pion, then relay media back to the browser via a localhost WebRTC loopback so the
browser still gets a real `MediaStream` and `<video>.srcObject` just works.

## Config toggle

Set `viewer.experimental_calls: true` in `goop.json` (requires restart).
When false (default), the existing browser WebRTC path is completely unchanged.

```json
{
  "viewer": {
    "experimental_calls": true
  }
}
```

## Architecture

```
[Camera/Mic]
     │
     ▼
pion/mediadevices (V4L2 / ALSA on Linux)
     │
     ├──────────────────────────────────────────────►  ExternalPC
     │                                               (Pion PeerConnection)
     │                                               ICE / DTLS / SRTP
     │                                               ◄──────────────────
     │                                            Remote peer
     │                                         (browser WebRTC or Pion)
     │
     └──────────────────────────────────────────────►  LocalPC
                                                    (Pion, localhost only)
                                                    ◄──────────────────
                                                    Browser webview
                                                    (RTCPeerConnection,
                                                     no getUserMedia)

Browser sets: videoElement.srcObject = remoteStream  ✓
```

### Two PeerConnections per session

| | ExternalPC | LocalPC |
|---|---|---|
| Purpose | Real P2P call | Browser display |
| ICE | STUN + relay | Loopback only |
| Media in | Local camera/mic tracks | Tracks relayed from ExternalPC |
| Media in | — | Local camera tracks (preview) |
| Media out | To remote peer | To browser `<video>` |

## Signaling

All P2P signaling goes through the existing realtime channel (unchanged).
Messages are **identical** to browser WebRTC — fully interoperable:

| Message | Direction |
|---|---|
| `call-request` | caller → callee |
| `call-ack` | callee → caller |
| `call-offer` | caller → callee (SDP) |
| `call-answer` | callee → caller (SDP) |
| `ice-candidate` | both directions |
| `call-hangup` | either peer |

Loopback signaling (browser ↔ Go, localhost only):

```
POST /api/call/loopback/{channel}/offer   browser SDP offer → Go answers
GET  /api/call/loopback/{channel}/ice     SSE: Go's loopback ICE candidates
POST /api/call/loopback/{channel}/ice     browser ICE candidates → Go
```

## API routes

```
GET  /api/call/mode           {"mode":"native"} or {"mode":"browser"}
POST /api/call/start          {channel_id, remote_peer}
POST /api/call/accept         {channel_id, remote_peer}
POST /api/call/hangup         {channel_id}
POST /api/call/toggle-audio   {channel_id} → {"muted":bool}
POST /api/call/toggle-video   {channel_id} → {"disabled":bool}
GET  /api/call/events         SSE: incoming call notifications
```

## Package structure

```
internal/call/
  types.go      Signaler interface, Envelope, IncomingCall
  manager.go    Session lifecycle, signaling dispatch
  session.go    ExternalPC + LocalPC, pion/mediadevices capture
internal/viewer/routes/call.go   HTTP route registration
internal/ui/assets/js/call-native.js   Frontend native-mode Goop.call
```

The `internal/call` package is **standalone** — it imports only Pion libraries
and stdlib. No imports from any other `internal/` package. Coupling to goop2 is
via the `Signaler` interface only.

## Go dependencies (Phase 3+)

```
github.com/pion/webrtc/v3
github.com/pion/mediadevices
github.com/pion/mediadevices/pkg/codec/vpx
github.com/pion/mediadevices/pkg/codec/opus
github.com/pion/mediadevices/pkg/driver/camera/v4l2
github.com/pion/mediadevices/pkg/driver/microphone/alsa
```

---

## Implementation Phases

### Phase 1 — Skeleton (compiles, flag works, no media yet)

**Status: ✅ Complete**

- [x] `internal/config/config.go`: add `ExperimentalCalls bool`
- [x] `internal/call/types.go`: Signaler interface, Envelope, IncomingCall
- [x] `internal/call/manager.go`: stub Manager (no Pion yet)
- [x] `internal/call/session.go`: stub Session (no Pion yet)
- [x] `internal/viewer/routes/call.go`: full route set (mode, start, accept, hangup, events, loopback stubs)
- [x] `internal/viewer/viewer.go`: add `Call` field
- [x] `internal/app/run.go`: wire call.Manager when experimental_calls=true
- [x] `internal/ui/templates/self.html`: add toggle in Calls section
- [x] `internal/viewer/routes/settings.go`: parse `viewer_experimental_calls`
- [x] `internal/ui/assets/js/call-native.js`: NativeSession + loopback stub
- [x] `internal/ui/assets/app.js`: add call-native.js

**Goal:** Build succeeds. Toggle visible in settings. `/api/call/mode` returns correct value.

---

### Phase 2 — Signaling bridge (calls ring, no media)

**Status: pending**

- [ ] `internal/call/manager.go`: verify dispatch handles all signaling types
- [ ] `internal/call/session.go`: add loopback ice candidate channel for SSE
- [ ] `internal/viewer/routes/call.go`: wire loopback ICE SSE to session
- [ ] `internal/ui/assets/js/call-native.js`: full loopback RTCPeerConnection setup
- [ ] Test: incoming call modal appears, accept/reject works, hangup cleans up

**Goal:** Incoming call modal appears on Linux. Accept/reject works. `call-hangup` cleans up session.

---

### Phase 3 — Media capture (local camera/mic flows to remote peer)

**Status: pending**

Prerequisites: Verify Phase 2 works end-to-end.

- [ ] `go get github.com/pion/webrtc/v3`
- [ ] `go get github.com/pion/mediadevices` + codec/vpx + codec/opus
- [ ] `go get github.com/pion/mediadevices/pkg/driver/camera/v4l2`
- [ ] `go get github.com/pion/mediadevices/pkg/driver/microphone/alsa`
- [ ] `internal/call/session.go`: add `ExternalPC *webrtc.PeerConnection`
- [ ] `internal/call/session.go`: open V4L2 camera + ALSA mic via mediadevices
- [ ] `internal/call/session.go`: add local tracks to ExternalPC
- [ ] Wire STUN servers for external ICE (e.g., `stun:stun.l.google.com:19302`)
- [ ] Wire ExternalPC ICE/SDP exchange through `handleSignal`

**Goal:** Linux peer sends camera+mic to a remote peer running browser WebRTC.

---

### Phase 4 — Loopback (browser displays remote video)

**Status: pending**

Prerequisites: Phase 3 working (ExternalPC sending media).

- [ ] `internal/call/session.go`: add `LocalPC *webrtc.PeerConnection` (loopback only)
- [ ] `internal/call/session.go`: `ExternalPC.OnTrack` → relay tracks to LocalPC
- [ ] `internal/call/session.go`: add local camera track to LocalPC for preview PiP
- [ ] `internal/call/session.go`: expose `LoopbackOffer(sdp) (string, error)` method
- [ ] `internal/call/session.go`: expose `AddLoopbackICE(candidate)` + ICE candidate channel
- [ ] `internal/viewer/routes/call.go`: wire `/loopback/{channel}/offer` to `session.LoopbackOffer`
- [ ] `internal/viewer/routes/call.go`: wire `/loopback/{channel}/ice` GET (SSE) and POST
- [ ] `internal/ui/assets/js/call-native.js`: complete loopback RTCPeerConnection
  - `createOffer` → POST `/offer` → `setRemoteDescription(answer)`
  - SSE `/ice` → `addIceCandidate`
  - POST own candidates → `/ice`

**Goal:** Remote peer's video appears in `<video>.srcObject` in the webview browser.

---

### Phase 5 — Polish

**Status: pending**

Prerequisites: Phase 4 working (full video loop).

- [ ] Local camera preview PiP (local track → LocalPC → browser)
- [ ] `ToggleAudio` / `ToggleVideo` work end-to-end (mute Pion track)
- [ ] Soft navigation survives calls (session outlives page navigation)
- [ ] Error handling: ICE failure, device not found, V4L2/ALSA device busy
- [ ] Cross-mode testing matrix:
  - Linux Pion → Linux Pion
  - Linux Pion → macOS browser WebRTC
  - Linux Pion → Windows browser WebRTC
- [ ] Update this doc with test results

---

## Signaling interoperability

The Pion ExternalPC sends and receives the same JSON payloads as browser WebRTC
through `realtime.Manager.Send()`:

| Field | Value |
|---|---|
| `{type:"call-request"}` | Initiator sends this to callee |
| `{type:"call-offer", sdp:"..."}` | SDP offer (caller → callee) |
| `{type:"call-answer", sdp:"..."}` | SDP answer (callee → caller) |
| `{type:"ice-candidate", candidate:{...}}` | Trickle ICE (both directions) |
| `{type:"call-hangup"}` | Either peer |

A remote peer running browser WebRTC sees no difference. Both peers can use
either path independently — the only shared protocol is the JSON signaling messages.

---

## Key files

| File | Role |
|---|---|
| `internal/config/config.go` | `ExperimentalCalls bool` field |
| `internal/call/types.go` | `Signaler`, `Envelope`, `IncomingCall` |
| `internal/call/manager.go` | Session lifecycle + signaling dispatch |
| `internal/call/session.go` | ExternalPC + LocalPC (Pion) |
| `internal/viewer/routes/call.go` | HTTP route registration |
| `internal/ui/assets/js/call-native.js` | Frontend native-mode `Goop.call` |
| `internal/app/run.go` | Wires `signalerAdapter` + creates `call.Manager` |
| `internal/viewer/viewer.go` | `Call *call.Manager` field |
| `docs/PION_WEBRTC.md` | This document |
