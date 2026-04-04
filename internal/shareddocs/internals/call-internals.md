# Call System Internals

## Overview

Native WebRTC call management using Pion (`github.com/pion/webrtc/v4`). The `internal/call/` package is maximally standalone — it imports only stdlib and Pion. Coupling to goop2 is via the `Signaler` interface only.

## Signaler interface

```go
type Signaler interface {
    RegisterChannel(channelID, peerID string)
    Send(channelID string, payload any) error
    Subscribe() (ch chan *Envelope, cancel func())
    PublishLocal(channelID string, payload any)
}
```

The concrete `mqSignalerAdapter` (in `app/modes/signaler.go`) bridges MQ to this interface. This is the only place that imports both packages.

## Manager

`call.Manager` owns active sessions and bridges signaling:

- `sessions map[string]*Session` — active calls keyed by channel ID
- `pendingCalls map[string]string` — received call-requests not yet accepted (channelID → origin peerID)
- `platform` — `runtime.GOOS`, sent in `call-ack` so the origin knows the call constellation

## Session

`call.Session` represents one active call between two peers:

- `channelID`, `remotePeer`, `isOrigin` (caller vs callee)
- `externalPC` — Pion PeerConnection to the remote peer
- `audioOn`, `videoOn` — mute/video state
- `hangupCh` — closed when call ends
- `mediaClose` — cleanup function for local media tracks

## Platform-specific media

Build-tagged files handle platform differences:

| File | Platform | Capabilities |
| -- | -- | -- |
| `media_linux.go` | Linux | VP8 + Opus via pion/mediadevices (V4L2 camera, malgo microphone) |
| `media_other.go` | Non-Linux | Receive-only PeerConnection (no local capture — browser WebRTC handles media) |

`initMediaPC(channelID, logFn)` returns the PeerConnection, media cleanup function, and optional SelfViewSource.

## Call flow

1. **Caller** sends `call-request` via MQ topic `call:{channelID}`
2. **Callee** receives request, stores in `pendingCalls`
3. **Callee** accepts → sends `call-ack` (includes platform for constellation detection)
4. **Caller** receives ack → creates ExternalPC → sends `call-offer` (SDP)
5. **Callee** receives offer → creates ExternalPC → sends `call-answer` (SDP)
6. **Both** exchange `ice-candidate` messages (trickle ICE)
7. **Either** sends `call-hangup` to end

## Phase 4: WebM streaming

`webm.go` — remote tracks are relayed to the browser via WebM stream:

- HTTP endpoint: `/api/call/media/{channel}` (or WebSocket on non-Linux)
- Linux: raw HTTP streaming of WebM container
- Other platforms: MSE (Media Source Extensions) in the browser feeds `<video>` element
- Avoids requiring RTCPeerConnection in the webview (important for WebKitGTK)

## Signal types

Defined in `internal/mq/topics.go`:

| Type | Direction | Purpose |
| -- | -- | -- |
| `call-request` | caller → callee | Initiate a call |
| `call-ack` | callee → caller | Call accepted |
| `call-offer` | caller → callee | SDP offer |
| `call-answer` | callee → caller | SDP answer |
| `ice-candidate` | either → other | Trickle ICE candidate |
| `call-hangup` | either side | End the call |
| `loopback-ice` | Go → browser | LocalPC ICE candidate (Phase 4) |
