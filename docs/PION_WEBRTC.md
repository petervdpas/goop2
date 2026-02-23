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

**Status: ✅ Complete**

- [x] `internal/call/manager.go`: `SubscribeIncoming/UnsubscribeIncoming` channel-based pub/sub; `call-ack` sent on accept; no callback-slice leak
- [x] `internal/call/session.go`: `hangupCh chan struct{}`, `HangupCh()`, `Hangup()` closes channel; `handleSignal` handles `call-hangup` + `call-ack`
- [x] `internal/viewer/routes/call.go`: `/api/call/events` SSE uses subscribe/defer-unsubscribe; `/api/call/session/{channel}/events` per-session hangup SSE
- [x] `internal/ui/assets/js/call-ui.js`: `Goop.callUI.showIncoming(info)` bridge for re-registration
- [x] `internal/ui/assets/js/video-call.js`: `window._callNativeMode` guard in `notifyIncoming` (suppresses double modal)
- [x] `internal/ui/assets/js/call-native.js`: `NativeSession` (sync toggles, `_listenForHangup`, `_connectLoopback` stub); `NativeCallManager` (SSE subscription, `start`, `onIncoming`); `init` sets flag + re-registers on new manager

**Goal:** Incoming call modal appears on Linux. Accept/reject works. `call-hangup` cleans up session.

---

### Phase 3 — Media capture (local camera/mic flows to remote peer)

**Status: ✅ Complete**

- [x] `go get github.com/pion/mediadevices v0.9.4` (pulls pion/webrtc/v4 — coexists with v3 used by libp2p)
- [x] `go get github.com/pion/mediadevices/pkg/codec/vpx` + `pkg/codec/opus`
- [x] `go get github.com/pion/mediadevices/pkg/driver/camera` (blackjack/webcam V4L2 backend)
- [x] `go get github.com/pion/mediadevices/pkg/driver/microphone` (gen2brain/malgo, cross-platform audio)
- [x] `internal/call/session.go`: full rewrite — ExternalPC with VP8+Opus codec selector, RegisterDefaultInterceptors, STUN server
- [x] `internal/call/session.go`: `initExternalPC()` goroutine — creates PC, captures camera+mic via GetUserMedia, adds tracks
- [x] `internal/call/session.go`: full SDP/ICE signal handling — `call-ack` → createAndSendOffer, `call-offer` → handleOffer, `call-answer` → handleAnswer, ICE candidate buffering until remote desc set
- [x] `internal/call/manager.go`: passes `isCaller bool` to `newSession` (true for StartCall, false for AcceptCall)
- [x] CI build scripts updated: `libvpx-dev libv4l-dev libopus-dev libasound2-dev` (Debian), `libvpx-devel libv4l-devel alsa-lib-devel opus-devel` (Fedora)
- [x] `.deb` control runtime Depends updated: `libvpx7, libv4l-0, libopus0, libasound2`
- [x] Flatpak finish-args: `--device=all`, `--socket=pulseaudio`

**Notes:**
- `pion/mediadevices/pkg/driver/microphone` uses `malgo` (miniaudio) — ALSA/PulseAudio on Linux, WASAPI on Windows. No separate ALSA dev package needed beyond `libasound2-dev`.
- `GetUserMedia` failure is non-fatal: PC is still created, call proceeds without local media (receive-only).
- `ToggleAudio`/`ToggleVideo` update local state only; actual Pion track muting is Phase 5.

**Goal:** Linux peer sends camera+mic to a remote peer running browser WebRTC.

---

### Phase 4 — Browser video display (WebM/MSE streaming)

**Status: complete**

> **Implementation note:** The original plan used an RTCPeerConnection loopback
> (Go LocalPC ↔ browser).  WebKitGTK/Wails v2 on Linux does not expose
> `RTCPeerConnection`, so that approach was replaced with WebM/MSE streaming.
> The result is simpler and works without any loopback PeerConnection.

Prerequisites: Phase 3 working (ExternalPC receiving remote media).

- [x] `internal/call/webm.go`: pure-Go EBML/WebM encoder + `webmSession` manager
  - EBML header, Segment (unknown size), Info, Tracks
  - VP8 video track (track 1) + Opus audio track (track 2)
  - `handleVideoFrame` / `handleAudioFrame` → live cluster assembly
  - `subscribeMedia` / `broadcastLocked` → per-subscriber WebSocket channels
- [x] `internal/call/session.go`: VP8/Opus depacketization + WebM streaming
  - `streamVideoTrack`: `codecs.VP8Packet.Unmarshal` + frame assembly on `Marker` bit
  - `streamAudioTrack`: Opus RTP payload → `webm.handleAudioFrame`
  - `SubscribeMedia() (<-chan []byte, func())` exposes the stream to HTTP layer
- [x] `internal/viewer/routes/call.go`: `/api/call/media/{channel}` WebSocket endpoint
  - Upgrades HTTP to WebSocket; streams binary WebM messages
  - Exits cleanly on hangup or client disconnect
- [x] `internal/ui/assets/js/call-native.js`: MSE path in `_connectLoopback`
  - Detects `RTCPeerConnection === undefined` → calls `_connectMSE()`
  - `_connectMSE()`: MediaSource + object URL → `onRemoteVideoSrc` + WebSocket append loop
  - Replay-on-subscribe for `onRemoteVideoSrc` (timing-safe)
- [x] `internal/ui/assets/js/call-ui.js`: `onRemoteVideoSrc` handler in `showActiveCall`
  - Sets `video.src = url` (not `srcObject`) for MSE streams

**Goal:** Remote peer's video appears in `<video>` element via MSE WebM stream.

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
