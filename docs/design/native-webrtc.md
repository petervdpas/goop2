# Native Go WebRTC Implementation

## Problem

WebKitGTK (used by Wails on Linux) does not properly support `getUserMedia()` for camera/microphone access. This is a known limitation that prevents video/audio calls from working on Linux.

**Error observed:**
```
NotAllowedError: The request is not allowed by the user agent or the platform in the current context
```

This is not a permissions issue - WebKitGTK simply doesn't implement media device access the same way as Chromium or Firefox.

**Affected platforms:**
- Linux (Wails uses WebKitGTK)
- Potentially other non-Chromium webviews

**Unaffected platforms:**
- Windows (Wails uses WebView2/Chromium)
- macOS (Wails uses WebKit, which has better support)

## Current Workaround

A `video_disabled` config option allows Linux users to broadcast to peers that they cannot participate in video/audio calls. Remote peers see a warning icon instead of call buttons.

```json
{
  "viewer": {
    "video_disabled": true
  }
}
```

This is a temporary solution - ideally all platforms should support calls.

## Proposed Solution

Move media capture and WebRTC handling to the Go backend using:

- **[pion/webrtc](https://github.com/pion/webrtc)** - Pure Go WebRTC implementation
- **[pion/mediadevices](https://github.com/pion/mediadevices)** - Go library for camera/microphone access

This bypasses the webview entirely for media handling.

## Architecture

### Current (Browser-Based)

```
┌─────────────────────────────────────────────────────┐
│                    Wails App                         │
│  ┌───────────────────────────────────────────────┐  │
│  │              WebView (WebKitGTK)               │  │
│  │                                                │  │
│  │   JavaScript                                   │  │
│  │   ├── getUserMedia() ──────► ✗ BLOCKED        │  │
│  │   ├── RTCPeerConnection                       │  │
│  │   └── UI rendering                            │  │
│  │                                                │  │
│  └───────────────────────────────────────────────┘  │
│                                                      │
│  ┌───────────────────────────────────────────────┐  │
│  │              Go Backend                        │  │
│  │   └── libp2p signaling                        │  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### Proposed (Go-Native)

```
┌─────────────────────────────────────────────────────┐
│                    Wails App                         │
│  ┌───────────────────────────────────────────────┐  │
│  │              WebView                           │  │
│  │                                                │  │
│  │   JavaScript                                   │  │
│  │   ├── UI only (buttons, layout)               │  │
│  │   └── <video> elements for display            │  │
│  │           ▲                                    │  │
│  │           │ Video frames via data URL or      │  │
│  │           │ WebSocket binary                  │  │
│  └───────────┼───────────────────────────────────┘  │
│              │                                       │
│  ┌───────────┼───────────────────────────────────┐  │
│  │           ▼      Go Backend                    │  │
│  │   ┌─────────────────────────────────────────┐ │  │
│  │   │ pion/mediadevices                       │ │  │
│  │   │   └── v4l2 camera access (Linux)        │ │  │
│  │   │   └── DirectShow (Windows)              │ │  │
│  │   │   └── AVFoundation (macOS)              │ │  │
│  │   └─────────────────────────────────────────┘ │  │
│  │                      │                         │  │
│  │                      ▼                         │  │
│  │   ┌─────────────────────────────────────────┐ │  │
│  │   │ pion/webrtc                             │ │  │
│  │   │   └── RTCPeerConnection                 │ │  │
│  │   │   └── ICE, DTLS, SRTP                   │ │  │
│  │   └─────────────────────────────────────────┘ │  │
│  │                      │                         │  │
│  │                      ▼                         │  │
│  │   ┌─────────────────────────────────────────┐ │  │
│  │   │ libp2p                                  │ │  │
│  │   │   └── Signaling (SDP, ICE candidates)   │ │  │
│  │   └─────────────────────────────────────────┘ │  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

## Implementation Approach

### 1. Media Capture in Go

```go
import (
    "github.com/pion/mediadevices"
    "github.com/pion/mediadevices/pkg/codec/vpx"    // VP8/VP9
    "github.com/pion/mediadevices/pkg/codec/opus"   // Opus audio
    "github.com/pion/mediadevices/pkg/driver"
    _ "github.com/pion/mediadevices/pkg/driver/camera"     // v4l2 on Linux
    _ "github.com/pion/mediadevices/pkg/driver/microphone" // ALSA/PulseAudio
)

func getMediaStream() (mediadevices.MediaStream, error) {
    // Register codecs
    mediadevices.RegisterDefaultCodecs()

    // Get user media (like browser getUserMedia)
    stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
        Video: func(c *mediadevices.MediaTrackConstraints) {
            c.Width = prop.Int(640)
            c.Height = prop.Int(480)
            c.FrameRate = prop.Float(30)
        },
        Audio: func(c *mediadevices.MediaTrackConstraints) {},
    })
    return stream, err
}
```

### 2. WebRTC in Go

```go
import "github.com/pion/webrtc/v3"

func createPeerConnection() (*webrtc.PeerConnection, error) {
    config := webrtc.Configuration{
        ICEServers: []webrtc.ICEServer{
            {URLs: []string{"stun:stun.l.google.com:19302"}},
        },
    }

    pc, err := webrtc.NewPeerConnection(config)
    if err != nil {
        return nil, err
    }

    // Add local tracks from mediadevices
    stream, _ := getMediaStream()
    for _, track := range stream.GetTracks() {
        pc.AddTrack(track)
    }

    return pc, nil
}
```

### 3. Signaling via libp2p

Use existing Goop P2P messaging for signaling:

```go
// In call handler
func handleCallSignaling(msg proto.CallMessage) {
    switch msg.Type {
    case "sdp_offer":
        // Set remote description, create answer
        pc.SetRemoteDescription(webrtc.SessionDescription{
            Type: webrtc.SDPTypeOffer,
            SDP:  msg.SDP,
        })
        answer, _ := pc.CreateAnswer(nil)
        pc.SetLocalDescription(answer)
        // Send answer back via libp2p

    case "ice_candidate":
        pc.AddICECandidate(webrtc.ICECandidateInit{
            Candidate: msg.Candidate,
        })
    }
}
```

### 4. Video Display in Webview

This is the challenging part. Options:

**Option A: WebSocket Binary Frames**
```go
// Go side - encode frames and send via WebSocket
func streamToWebview(track *webrtc.TrackRemote) {
    for {
        rtp, _, _ := track.ReadRTP()
        // Decode RTP to raw frame
        // Encode as JPEG
        // Send via WebSocket to webview
    }
}
```
```javascript
// JavaScript side
const ws = new WebSocket('ws://localhost:PORT/video-stream');
ws.binaryType = 'arraybuffer';
ws.onmessage = (e) => {
    const blob = new Blob([e.data], {type: 'image/jpeg'});
    videoElement.src = URL.createObjectURL(blob);
};
```

**Option B: Local HTTP MJPEG Stream**
```go
// Serve MJPEG stream on local HTTP
func mjpegHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
    for frame := range frames {
        w.Write([]byte("--frame\r\n"))
        w.Write([]byte("Content-Type: image/jpeg\r\n\r\n"))
        w.Write(frame)
        w.Write([]byte("\r\n"))
    }
}
```
```html
<img src="http://localhost:PORT/video-stream/remote">
```

**Option C: Canvas + Data URLs**
```javascript
// Receive base64 frames, draw to canvas
Goop.call.onFrame = (base64) => {
    const img = new Image();
    img.onload = () => ctx.drawImage(img, 0, 0);
    img.src = 'data:image/jpeg;base64,' + base64;
};
```

## Dependencies

```
go get github.com/pion/webrtc/v3
go get github.com/pion/mediadevices
```

### Linux Build Requirements

```bash
# For camera (v4l2)
sudo apt install libv4l-dev

# For audio (PulseAudio)
sudo apt install libpulse-dev

# For VP8/VP9 encoding
sudo apt install libvpx-dev

# For Opus audio
sudo apt install libopus-dev
```

### CGO Note

pion/mediadevices requires CGO for native device access:

```bash
CGO_ENABLED=1 go build
```

## Challenges

### 1. Frame Transport Latency

Sending decoded frames through WebSocket/HTTP adds latency compared to native `<video>` playback. May need to:
- Use efficient encoding (MJPEG vs VP8 re-encode)
- Optimize frame size
- Consider lower resolution for remote video

### 2. Audio Routing

Audio is trickier than video - can't easily pipe through HTTP. Options:
- Play audio directly from Go (using portaudio or similar)
- WebSocket Audio API (experimental)
- Keep audio separate from video display

### 3. Cross-Platform Builds

Different drivers per platform:
- Linux: v4l2, PulseAudio/ALSA
- Windows: DirectShow, WASAPI
- macOS: AVFoundation

Build tags and conditional compilation needed.

### 4. Resource Management

Must properly cleanup:
- MediaDevices streams
- PeerConnections
- Goroutines for frame encoding

## Alternative: Hybrid Approach

Use browser WebRTC where it works (Windows/macOS), Go-native only on Linux:

```go
func supportsNativeWebRTC() bool {
    return runtime.GOOS == "linux"
}

// Expose different APIs to frontend based on platform
```

This minimizes complexity while solving the Linux problem.

## Proof of Concept Steps

1. **Camera capture test**: Get pion/mediadevices capturing from v4l2
2. **MJPEG stream**: Serve captured frames via HTTP
3. **Display in webview**: Show MJPEG stream in `<img>` tag
4. **WebRTC integration**: Add pion/webrtc peer connection
5. **Signaling**: Wire up to existing libp2p messaging
6. **Remote video**: Decode incoming RTP and stream to webview

## References

- [pion/webrtc examples](https://github.com/pion/webrtc/tree/master/examples)
- [pion/mediadevices docs](https://github.com/pion/mediadevices)
- [v4l2 on Linux](https://www.kernel.org/doc/html/latest/userspace-api/media/v4l/v4l2.html)
- [Wails + WebRTC discussion](https://github.com/pion/mediadevices/issues/317)
