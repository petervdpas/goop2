package call

// webm.go — minimal WebM/EBML encoder and streaming session for Phase 4.
//
// No external dependencies — pure Go EBML encoding.
//
// The output is a live WebM stream containing VP8 video and (optionally) Opus audio.
// Each cluster is one self-contained binary message sent to WebSocket subscribers
// via webmSession.  The first message is always the init segment (EBML header +
// Segment start + Info + Tracks), followed by clusters.
//
// MSE (Media Source Extensions) on the browser side receives these messages
// and feeds them to a <video> element, giving us remote video display without
// requiring RTCPeerConnection in the webview.

import (
	"bytes"
	"encoding/binary"
	"log"
	"math"
	"sync"
)

// ─── EBML encoding helpers ───────────────────────────────────────────────────

// ebmlVint encodes v as an EBML variable-length integer for element sizes.
// Valid range: 0..268435454 (4-byte max, sufficient for any reasonable WebM element).
func ebmlVint(v uint64) []byte {
	switch {
	case v < 0x7F: // 1 byte: 0xxxxxxx → 1xxxxxxx
		return []byte{byte(0x80 | v)}
	case v < 0x3FFF: // 2 bytes
		return []byte{byte(0x40 | (v >> 8)), byte(v)}
	case v < 0x1FFFFF: // 3 bytes
		return []byte{byte(0x20 | (v >> 16)), byte(v >> 8), byte(v)}
	default: // 4 bytes
		return []byte{byte(0x10 | (v >> 24)), byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

// ebmlUnkSize is the 8-byte unknown-size marker used for streaming Segment and Cluster
// elements whose length is not known at write time.
var ebmlUnkSize = []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

// ebmlElem encodes an EBML element: id bytes + vint(len(data)) + data.
func ebmlElem(id, data []byte) []byte {
	b := make([]byte, 0, len(id)+8+len(data))
	b = append(b, id...)
	b = append(b, ebmlVint(uint64(len(data)))...)
	return append(b, data...)
}

// ebmlUint encodes an unsigned integer in the minimal number of big-endian bytes.
func ebmlUint(v uint64) []byte {
	if v == 0 {
		return []byte{0}
	}
	n := 0
	for x := v; x > 0; x >>= 8 {
		n++
	}
	b := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		b[i] = byte(v)
		v >>= 8
	}
	return b
}

// ebmlConcat joins byte slices efficiently.
func ebmlConcat(slices ...[]byte) []byte {
	n := 0
	for _, s := range slices {
		n += len(s)
	}
	b := make([]byte, 0, n)
	for _, s := range slices {
		b = append(b, s...)
	}
	return b
}

// ─── Element IDs ─────────────────────────────────────────────────────────────

var (
	idEBML         = []byte{0x1A, 0x45, 0xDF, 0xA3}
	idEBMLVersion  = []byte{0x42, 0x86}
	idEBMLReadVer  = []byte{0x42, 0xF7}
	idEBMLMaxIDLen = []byte{0x42, 0xF2}
	idEBMLMaxSzLen = []byte{0x42, 0xF3}
	idDocType      = []byte{0x42, 0x82}
	idDocTypeVer   = []byte{0x42, 0x87}
	idDocTypeRdVer = []byte{0x42, 0x85}
	idSegment      = []byte{0x18, 0x53, 0x80, 0x67}
	idInfo         = []byte{0x15, 0x49, 0xA9, 0x66}
	idTcScale      = []byte{0x2A, 0xD7, 0xB1}
	idMuxApp       = []byte{0x4D, 0x80}
	idWrtApp       = []byte{0x57, 0x41}
	idTracks       = []byte{0x16, 0x54, 0xAE, 0x6B}
	idTrackEntry   = []byte{0xAE}
	idTrackNum     = []byte{0xD7}
	idTrackUID     = []byte{0x73, 0xC5}
	idTrackType    = []byte{0x83}
	idCodecID      = []byte{0x86}
	idCodecPrv     = []byte{0x63, 0xA2}
	idVideo        = []byte{0xE0}
	idPixelW       = []byte{0xB0}
	idPixelH       = []byte{0xBA}
	idAudio        = []byte{0xE1}
	idSampFreq     = []byte{0xB5}
	idChannels     = []byte{0x9F}
	idCluster      = []byte{0x1F, 0x43, 0xB6, 0x75}
	idTimecode     = []byte{0xE7}
	idSimpleBlock  = []byte{0xA3}
)

// opusHead is the codec private data (OpusHead) for stereo 48 kHz Opus.
// Required by WebM for Opus audio tracks.
var opusHead = []byte{
	'O', 'p', 'u', 's', 'H', 'e', 'a', 'd', // magic
	0x01,                   // version = 1
	0x01,                   // channels = 1 (mono)
	0x38, 0x01,             // pre-skip = 312 (LE)
	0x80, 0xBB, 0x00, 0x00, // input sample rate = 48000 (LE)
	0x00, 0x00,             // output gain = 0 (LE)
	0x00,                   // channel mapping family = 0
}

// webmInitSegment returns the WebM initialisation segment:
// EBML header + Segment (unknown size) + Info + Tracks.
// withAudio=true adds an Opus audio track (track 2) alongside VP8 video (track 1).
// videoW/videoH are the video pixel dimensions.
func webmInitSegment(videoW, videoH uint16, withAudio bool) []byte {
	var buf bytes.Buffer

	// EBML header element
	ebmlBody := ebmlConcat(
		ebmlElem(idEBMLVersion, ebmlUint(1)),
		ebmlElem(idEBMLReadVer, ebmlUint(1)),
		ebmlElem(idEBMLMaxIDLen, ebmlUint(4)),
		ebmlElem(idEBMLMaxSzLen, ebmlUint(8)),
		ebmlElem(idDocType, []byte("webm")),
		ebmlElem(idDocTypeVer, ebmlUint(2)),
		ebmlElem(idDocTypeRdVer, ebmlUint(2)),
	)
	buf.Write(ebmlElem(idEBML, ebmlBody))

	// Segment with unknown size (streaming)
	buf.Write(idSegment)
	buf.Write(ebmlUnkSize)

	// SegmentInfo
	infoBody := ebmlConcat(
		ebmlElem(idTcScale, ebmlUint(1000000)), // 1 ms per timecode unit
		ebmlElem(idMuxApp, []byte("goop2")),
		ebmlElem(idWrtApp, []byte("goop2")),
	)
	buf.Write(ebmlElem(idInfo, infoBody))

	// Video track (track 1, VP8)
	videoBody := ebmlConcat(
		ebmlElem(idPixelW, ebmlUint(uint64(videoW))),
		ebmlElem(idPixelH, ebmlUint(uint64(videoH))),
	)
	videoEntry := ebmlConcat(
		ebmlElem(idTrackNum, ebmlUint(1)),
		ebmlElem(idTrackUID, ebmlUint(1)),
		ebmlElem(idTrackType, ebmlUint(1)), // 1 = video
		ebmlElem(idCodecID, []byte("V_VP8")),
		ebmlElem(idVideo, videoBody),
	)
	tracksBody := ebmlElem(idTrackEntry, videoEntry)

	if withAudio {
		// SamplingFrequency: 4-byte IEEE 754 float
		freqBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(freqBytes, math.Float32bits(48000.0))
		audioBody := ebmlConcat(
			ebmlElem(idSampFreq, freqBytes),
			ebmlElem(idChannels, ebmlUint(1)),
		)
		audioEntry := ebmlConcat(
			ebmlElem(idTrackNum, ebmlUint(2)),
			ebmlElem(idTrackUID, ebmlUint(2)),
			ebmlElem(idTrackType, ebmlUint(2)), // 2 = audio
			ebmlElem(idCodecID, []byte("A_OPUS")),
			ebmlElem(idCodecPrv, opusHead),
			ebmlElem(idAudio, audioBody),
		)
		tracksBody = ebmlConcat(tracksBody, ebmlElem(idTrackEntry, audioEntry))
	}
	buf.Write(ebmlElem(idTracks, tracksBody))
	return buf.Bytes()
}

// webmCluster builds a complete Cluster binary message containing the given
// SimpleBlock entries.  clusterMs is the cluster's absolute timecode in ms.
// blocks is pre-encoded SimpleBlock elements (as returned by webmSimpleBlock).
func webmCluster(clusterMs int64, blocks []byte) []byte {
	tcElem := ebmlElem(idTimecode, ebmlUint(uint64(clusterMs)))
	clusterBody := ebmlConcat(tcElem, blocks)
	// Use known size so MSE doesn't have to scan for the next cluster start.
	return ebmlElem(idCluster, clusterBody)
}

// webmSimpleBlock encodes a single SimpleBlock element.
// trackNum: 1 = video, 2 = audio
// relMs: timecode relative to cluster start (signed int16, clamped to ±32767)
// keyframe: true for keyframes (flags = 0x80), false for delta frames (flags = 0x00)
func webmSimpleBlock(trackNum int, relMs int16, keyframe bool, data []byte) []byte {
	trackVint := ebmlVint(uint64(trackNum))
	var flags byte
	if keyframe {
		flags = 0x80
	}
	content := make([]byte, len(trackVint)+2+1+len(data))
	copy(content, trackVint)
	binary.BigEndian.PutUint16(content[len(trackVint):], uint16(relMs))
	content[len(trackVint)+2] = flags
	copy(content[len(trackVint)+3:], data)
	return ebmlElem(idSimpleBlock, content)
}

// ─── webmSession ─────────────────────────────────────────────────────────────

// webmSession manages the live WebM stream for one call session.
// Video and audio goroutines call handleVideoFrame / handleAudioFrame.
// HTTP WebSocket handlers subscribe via subscribeMedia / unsubscribeMedia.
type webmSession struct {
	mu        sync.Mutex
	channelID string // for log messages

	// Video track state
	dimKnown    bool
	videoWidth  uint16
	videoHeight uint16
	hasAudio    bool // set before first frame if an audio track was announced

	// Init segment (nil until first keyframe with known dimensions)
	initSeg []byte

	// Last keyframe cluster — replayed to new subscribers so they always
	// start from a clean VP8 decode state instead of receiving P-frames
	// mid-stream and producing garbled video.
	lastKeyCluster []byte
	clusterIsKey   bool // true when current open cluster started with a keyframe

	// Cluster accumulation
	clusterStartMs int64
	clusterBlocks  bytes.Buffer
	clusterOpen    bool

	// Audio frames queued between video frames; drained into each video cluster.
	// Unbounded — ensures no audio is dropped regardless of camera frame rate.
	audioQ []webmAudioFrame

	// Subscriber channels: each receives binary WebSocket messages
	subs map[chan []byte]struct{}

	// Timestamp normalization: first frame of each track becomes t=0.
	// VP8 and Opus RTP clocks start at independent random values; without
	// normalization the cluster timecodes are huge (hours) and audio relMs
	// values are millions of ms off, causing silent MSE rejection.
	baseVideoMs  int64
	baseVideoSet bool
	baseAudioMs  int64
	baseAudioSet bool
}

type webmAudioFrame struct {
	timecodeMs int64
	data       []byte
}

func newWebmSession(channelID string) *webmSession {
	return &webmSession{
		channelID: channelID,
		subs:      make(map[chan []byte]struct{}),
	}
}

// enableAudio marks that an audio track will be included in the stream.
// Must be called before the first video frame.
func (ws *webmSession) enableAudio() {
	ws.mu.Lock()
	ws.hasAudio = true
	ws.mu.Unlock()
}

// hasInitSeg reports whether the init segment has been generated (i.e. the
// first VP8 keyframe has been received and its dimensions are known).
func (ws *webmSession) hasInitSeg() bool {
	ws.mu.Lock()
	ok := ws.initSeg != nil
	ws.mu.Unlock()
	return ok
}

// subscribeMedia returns a channel that receives WebM binary messages and a
// cancel function.  The first message sent on the channel is the init segment
// (if it has already been produced); subsequent messages are clusters.
func (ws *webmSession) subscribeMedia() (<-chan []byte, func()) {
	ch := make(chan []byte, 32)
	ws.mu.Lock()
	replayed := ws.initSeg != nil
	if replayed {
		// Send cached init segment immediately to the new subscriber.
		select {
		case ch <- ws.initSeg:
		default:
		}
		// Also replay the last keyframe cluster so the VP8 decoder starts from
		// a clean reference frame.  Without this, subscribers that join
		// mid-stream (page navigations, WebSocket reconnects) receive P-frames
		// and the video is garbled until the next natural keyframe arrives.
		if ws.lastKeyCluster != nil {
			select {
			case ch <- ws.lastKeyCluster:
			default:
			}
		}
	}
	ws.subs[ch] = struct{}{}
	n := len(ws.subs)
	ws.mu.Unlock()
	log.Printf("CALL [%s]: WebM subscriber added (total=%d, init_replayed=%v)", ws.channelID, n, replayed)
	return ch, func() {
		ws.mu.Lock()
		delete(ws.subs, ch)
		n := len(ws.subs)
		ws.mu.Unlock()
		close(ch)
		log.Printf("CALL [%s]: WebM subscriber removed (total=%d)", ws.channelID, n)
	}
}

// handleVideoFrame is called from the VP8 streaming goroutine.
// One cluster per frame, flushed immediately.  Any audio frames that arrived
// since the last flush are drained from the queue into the cluster before the
// video block, so GStreamer always receives a well-formed audio+video cluster.
func (ws *webmSession) handleVideoFrame(timecodeMs int64, keyframe bool, data []byte) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Normalize so the first video frame is at t=0ms.  VP8 RTP timestamps
	// start at a large random value; without this the cluster timecodes would
	// be millions of ms into the future and MSE would silently discard all data.
	if !ws.baseVideoSet {
		ws.baseVideoMs = timecodeMs
		ws.baseVideoSet = true
	}
	tsMs := timecodeMs - ws.baseVideoMs

	// Extract video dimensions from the first VP8 keyframe header.
	if !ws.dimKnown && keyframe && len(data) >= 10 {
		if data[3] == 0x9D && data[4] == 0x01 && data[5] == 0x2A {
			ws.videoWidth = binary.LittleEndian.Uint16(data[6:8]) & 0x3FFF
			ws.videoHeight = binary.LittleEndian.Uint16(data[8:10]) & 0x3FFF
		} else {
			ws.videoWidth = 640 // fallback
			ws.videoHeight = 480
		}
		ws.dimKnown = true
	}

	// Send init segment on first keyframe.
	if ws.initSeg == nil {
		if !ws.dimKnown || !keyframe {
			return // wait for a keyframe so we know dimensions and MSE can start
		}
		ws.initSeg = webmInitSegment(ws.videoWidth, ws.videoHeight, ws.hasAudio)
		log.Printf("CALL [%s]: WebM init segment — VP8 %dx%d audio=%v subs=%d",
			ws.channelID, ws.videoWidth, ws.videoHeight, ws.hasAudio, len(ws.subs))
		ws.broadcastLocked(ws.initSeg)
	}

	// Start a new cluster at each keyframe (seekable boundary point).
	if keyframe && ws.clusterOpen {
		ws.flushClusterLocked()
	}

	if !ws.clusterOpen {
		// Anchor the cluster to the earliest queued audio frame so that all
		// audio SimpleBlocks have positive (or zero) relative timecodes.
		// GStreamer/WebKitGTK handles positive-relative audio better than
		// large negative values that predate the cluster's video timestamp.
		ws.clusterStartMs = tsMs
		if len(ws.audioQ) > 0 && ws.audioQ[0].timecodeMs < tsMs {
			ws.clusterStartMs = ws.audioQ[0].timecodeMs
		}
		ws.clusterOpen = true
		ws.clusterIsKey = keyframe
		ws.clusterBlocks.Reset()

		// Drain queued audio frames into this cluster.
		newQ := ws.audioQ[:0]
		for _, af := range ws.audioQ {
			rel := af.timecodeMs - ws.clusterStartMs
			if rel < -30000 || rel > 30000 {
				continue
			}
			ws.clusterBlocks.Write(webmSimpleBlock(2, int16(rel), false, af.data))
		}
		ws.audioQ = newQ
	}

	relMs := int16(tsMs - ws.clusterStartMs)
	ws.clusterBlocks.Write(webmSimpleBlock(1, relMs, keyframe, data))
	ws.flushClusterLocked()
}

// handleAudioFrame is called from the Opus streaming goroutine.
func (ws *webmSession) handleAudioFrame(timecodeMs int64, data []byte) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Normalize so the first audio frame is at t=0ms.  Opus and VP8 RTP clocks
	// have independent random starting offsets; both are normalized to 0 so that
	// relMs values within a cluster are small (within a frame interval).
	if !ws.baseAudioSet {
		ws.baseAudioMs = timecodeMs
		ws.baseAudioSet = true
	}
	tsMs := timecodeMs - ws.baseAudioMs

	// Queue audio until the next video frame opens a cluster and drains it.
	// No cap — at any video fps, all audio is preserved and delivered as part
	// of the next video cluster.  GStreamer always sees video+audio clusters,
	// which prevents stalls on the video track regardless of camera frame rate.
	ws.audioQ = append(ws.audioQ, webmAudioFrame{tsMs, data})
}

// flushClusterLocked builds a Cluster message from accumulated blocks and
// broadcasts it.  Must be called with ws.mu held.
func (ws *webmSession) flushClusterLocked() {
	if !ws.clusterOpen || ws.clusterBlocks.Len() == 0 {
		ws.clusterOpen = false
		return
	}
	cluster := webmCluster(ws.clusterStartMs, ws.clusterBlocks.Bytes())
	// Cache keyframe clusters so new subscribers (page navigations, reconnects)
	// always start from a clean VP8 decode state instead of receiving P-frames
	// and producing garbled video.
	if ws.clusterIsKey {
		ws.lastKeyCluster = cluster
	}
	ws.clusterOpen = false
	ws.clusterIsKey = false
	ws.clusterBlocks.Reset()
	ws.broadcastLocked(cluster)
}

// broadcastLocked sends data to all subscribers, dropping slow ones.
// Must be called with ws.mu held.
func (ws *webmSession) broadcastLocked(data []byte) {
	for ch := range ws.subs {
		select {
		case ch <- data:
		default: // subscriber too slow — drop frame, don't block
		}
	}
}
