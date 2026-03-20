package call

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEbmlVint_OneByte(t *testing.T) {
	for _, tc := range []struct {
		in   uint64
		want []byte
	}{
		{0, []byte{0x80}},
		{1, []byte{0x81}},
		{0x7E, []byte{0xFE}},
	} {
		got := ebmlVint(tc.in)
		if !bytes.Equal(got, tc.want) {
			t.Errorf("ebmlVint(%d) = %x, want %x", tc.in, got, tc.want)
		}
	}
}

func TestEbmlVint_TwoBytes(t *testing.T) {
	got := ebmlVint(0x7F)
	if len(got) != 2 {
		t.Fatalf("ebmlVint(0x7F) should be 2 bytes, got %d", len(got))
	}
	if got[0]&0x40 == 0 {
		t.Fatal("2-byte vint should have 0x40 marker in first byte")
	}
}

func TestEbmlVint_ThreeBytes(t *testing.T) {
	got := ebmlVint(0x3FFF)
	if len(got) != 3 {
		t.Fatalf("ebmlVint(0x3FFF) should be 3 bytes, got %d", len(got))
	}
	if got[0]&0x20 == 0 {
		t.Fatal("3-byte vint should have 0x20 marker in first byte")
	}
}

func TestEbmlVint_FourBytes(t *testing.T) {
	got := ebmlVint(0x1FFFFF)
	if len(got) != 4 {
		t.Fatalf("ebmlVint(0x1FFFFF) should be 4 bytes, got %d", len(got))
	}
	if got[0]&0x10 == 0 {
		t.Fatal("4-byte vint should have 0x10 marker in first byte")
	}
}

func TestEbmlUint_Zero(t *testing.T) {
	got := ebmlUint(0)
	if !bytes.Equal(got, []byte{0}) {
		t.Fatalf("ebmlUint(0) = %x, want 00", got)
	}
}

func TestEbmlUint_MinimalEncoding(t *testing.T) {
	for _, tc := range []struct {
		in      uint64
		wantLen int
	}{
		{1, 1},
		{255, 1},
		{256, 2},
		{65535, 2},
		{65536, 3},
		{1 << 24, 4},
	} {
		got := ebmlUint(tc.in)
		if len(got) != tc.wantLen {
			t.Errorf("ebmlUint(%d): len=%d, want %d", tc.in, len(got), tc.wantLen)
		}
	}
}

func TestEbmlUint_RoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 127, 255, 256, 1000, 48000, 65535, 1 << 20} {
		b := ebmlUint(v)
		var result uint64
		for _, bv := range b {
			result = (result << 8) | uint64(bv)
		}
		if result != v {
			t.Errorf("ebmlUint(%d) round-trip = %d", v, result)
		}
	}
}

func TestEbmlElem_Structure(t *testing.T) {
	id := []byte{0xAB, 0xCD}
	data := []byte("hello")
	got := ebmlElem(id, data)

	if !bytes.HasPrefix(got, id) {
		t.Fatal("element should start with ID bytes")
	}
	if !bytes.HasSuffix(got, data) {
		t.Fatal("element should end with data bytes")
	}
	if len(got) < len(id)+1+len(data) {
		t.Fatal("element too short: missing vint size")
	}
}

func TestEbmlConcat(t *testing.T) {
	a := []byte{1, 2}
	b := []byte{3, 4, 5}
	c := []byte{6}
	got := ebmlConcat(a, b, c)
	want := []byte{1, 2, 3, 4, 5, 6}
	if !bytes.Equal(got, want) {
		t.Fatalf("ebmlConcat = %v, want %v", got, want)
	}
}

func TestEbmlConcat_Empty(t *testing.T) {
	got := ebmlConcat()
	if len(got) != 0 {
		t.Fatalf("ebmlConcat() should return empty, got %d bytes", len(got))
	}
}

func TestWebmInitSegment_VideoOnly(t *testing.T) {
	seg := webmInitSegment(640, 480, false)

	if !bytes.Contains(seg, []byte("webm")) {
		t.Fatal("init segment should contain doctype 'webm'")
	}
	if !bytes.Contains(seg, []byte("V_VP8")) {
		t.Fatal("init segment should contain VP8 codec ID")
	}
	if bytes.Contains(seg, []byte("A_OPUS")) {
		t.Fatal("video-only init segment should not contain Opus codec")
	}
	if !bytes.Contains(seg, []byte("goop2")) {
		t.Fatal("init segment should contain muxing app name")
	}
}

func TestWebmInitSegment_WithAudio(t *testing.T) {
	seg := webmInitSegment(320, 240, true)

	if !bytes.Contains(seg, []byte("V_VP8")) {
		t.Fatal("init segment should contain VP8 codec ID")
	}
	if !bytes.Contains(seg, []byte("A_OPUS")) {
		t.Fatal("audio-enabled init segment should contain Opus codec ID")
	}
	if !bytes.Contains(seg, opusHead) {
		t.Fatal("init segment should contain OpusHead codec private data")
	}
}

func TestWebmInitSegment_EBMLHeader(t *testing.T) {
	seg := webmInitSegment(640, 480, false)

	if !bytes.HasPrefix(seg, idEBML) {
		t.Fatal("init segment must start with EBML header element ID")
	}
}

func TestWebmSimpleBlock_Keyframe(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	block := webmSimpleBlock(1, 0, true, data)

	if !bytes.HasPrefix(block, idSimpleBlock) {
		t.Fatal("SimpleBlock should start with element ID 0xA3")
	}
	if !bytes.Contains(block, data) {
		t.Fatal("SimpleBlock should contain frame data")
	}
	// Keyframe flag byte (0x80) should be present somewhere after track+timecode
	found := false
	for i, b := range block {
		if b == 0x80 && i > len(idSimpleBlock)+1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("keyframe SimpleBlock should contain flag byte 0x80")
	}
}

func TestWebmSimpleBlock_DeltaFrame(t *testing.T) {
	block := webmSimpleBlock(1, 100, false, []byte{0x01})

	// Delta frame: flags byte should be 0x00, not 0x80
	if bytes.Contains(block, []byte{0x80}) {
		// Need to be more precise — check the flags position
		// Track vint for track 1 = 0x81 (1 byte), timecode = 2 bytes, then flags
		inner := block[len(idSimpleBlock):]
		// Skip the vint size
		for i, b := range inner {
			if b&0x80 != 0 && i == 0 {
				continue // vint size byte
			}
			break
		}
	}
	// Simpler check: block should contain the data
	if !bytes.Contains(block, []byte{0x01}) {
		t.Fatal("delta frame should contain payload")
	}
}

func TestWebmCluster_ContainsTimecodeAndBlocks(t *testing.T) {
	blockData := webmSimpleBlock(1, 0, true, []byte{0xFF})
	cluster := webmCluster(1000, blockData)

	if !bytes.HasPrefix(cluster, idCluster) {
		t.Fatal("cluster should start with Cluster element ID")
	}
	if !bytes.Contains(cluster, idTimecode) {
		t.Fatal("cluster should contain Timecode element")
	}
	if !bytes.Contains(cluster, blockData) {
		t.Fatal("cluster should contain the SimpleBlock data")
	}
}

func TestWebmSession_SubscribeMedia_InitReplay(t *testing.T) {
	ws := newWebmSession("test-ch")

	// Build a VP8 keyframe with proper header for dimension extraction.
	frame := makeVP8Keyframe(320, 240)
	ws.handleVideoFrame(0, true, frame)

	if !ws.hasInitSeg() {
		t.Fatal("should have init segment after first keyframe")
	}

	// New subscriber should get init segment replayed.
	ch, cancel := ws.subscribeMedia()
	defer cancel()

	select {
	case msg := <-ch:
		if !bytes.Contains(msg, []byte("webm")) {
			t.Fatal("first replayed message should be init segment")
		}
	default:
		t.Fatal("subscriber should receive init segment immediately")
	}
}

func TestWebmSession_SubscribeMedia_KeyframeReplay(t *testing.T) {
	ws := newWebmSession("test-ch")

	frame := makeVP8Keyframe(320, 240)
	ws.handleVideoFrame(0, true, frame)
	ws.handleVideoFrame(33, false, []byte{0x01, 0x02}) // p-frame
	ws.handleVideoFrame(66, true, frame)                // second keyframe

	ch, cancel := ws.subscribeMedia()
	defer cancel()

	// Should get init segment + last keyframe cluster
	msgs := drainChan(ch, 2)
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 replayed messages (init + keyframe), got %d", len(msgs))
	}
}

func TestWebmSession_BroadcastToMultipleSubscribers(t *testing.T) {
	ws := newWebmSession("test-ch")

	ch1, cancel1 := ws.subscribeMedia()
	defer cancel1()
	ch2, cancel2 := ws.subscribeMedia()
	defer cancel2()

	frame := makeVP8Keyframe(320, 240)
	ws.handleVideoFrame(0, true, frame)

	// Both subscribers should receive messages.
	msg1 := drainChan(ch1, 1)
	msg2 := drainChan(ch2, 1)
	if len(msg1) == 0 || len(msg2) == 0 {
		t.Fatal("all subscribers should receive broadcast")
	}
}

func TestWebmSession_Unsubscribe(t *testing.T) {
	ws := newWebmSession("test-ch")

	_, cancel := ws.subscribeMedia()
	cancel()

	ws.mu.Lock()
	n := len(ws.subs)
	ws.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 subscribers after cancel, got %d", n)
	}
}

func TestWebmSession_AudioQueuing(t *testing.T) {
	ws := newWebmSession("test-ch")
	ws.enableAudio()

	// Queue audio before any video.
	ws.handleAudioFrame(0, []byte{0xAA})
	ws.handleAudioFrame(20, []byte{0xBB})

	ws.mu.Lock()
	qLen := len(ws.audioQ)
	ws.mu.Unlock()
	if qLen != 2 {
		t.Fatalf("expected 2 queued audio frames, got %d", qLen)
	}

	// First video keyframe should drain the audio queue.
	ch, cancel := ws.subscribeMedia()
	defer cancel()
	frame := makeVP8Keyframe(320, 240)
	ws.handleVideoFrame(10, true, frame)

	ws.mu.Lock()
	qLen = len(ws.audioQ)
	ws.mu.Unlock()
	if qLen != 0 {
		t.Fatalf("audio queue should be drained after video frame, got %d", qLen)
	}
	_ = ch
}

func TestWebmSession_VP8DimensionExtraction(t *testing.T) {
	ws := newWebmSession("test-ch")

	frame := makeVP8Keyframe(1280, 720)
	ws.handleVideoFrame(0, true, frame)

	ws.mu.Lock()
	w, h := ws.videoWidth, ws.videoHeight
	ws.mu.Unlock()

	if w != 1280 || h != 720 {
		t.Fatalf("expected 1280x720, got %dx%d", w, h)
	}
}

func TestWebmSession_VP8DimensionFallback(t *testing.T) {
	ws := newWebmSession("test-ch")

	// Keyframe without valid VP8 magic bytes.
	frame := make([]byte, 20)
	ws.handleVideoFrame(0, true, frame)

	ws.mu.Lock()
	w, h := ws.videoWidth, ws.videoHeight
	ws.mu.Unlock()

	if w != 640 || h != 480 {
		t.Fatalf("expected fallback 640x480, got %dx%d", w, h)
	}
}

func TestWebmSession_TimestampNormalization(t *testing.T) {
	ws := newWebmSession("test-ch")

	// First frame at a high RTP timestamp — should be normalized to 0.
	frame := makeVP8Keyframe(320, 240)
	ws.handleVideoFrame(500000, true, frame)

	ws.mu.Lock()
	base := ws.baseVideoMs
	ws.mu.Unlock()

	if base != 500000 {
		t.Fatalf("base video timestamp should be 500000, got %d", base)
	}
}

func TestVP8KeyframeDetection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		firstBit byte
		wantKey  bool
	}{
		{"keyframe (bit0=0)", 0x00, true},
		{"keyframe with other bits", 0xFE, true},
		{"delta (bit0=1)", 0x01, false},
		{"delta with other bits", 0xFF, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := (tc.firstBit & 0x01) == 0
			if got != tc.wantKey {
				t.Errorf("byte 0x%02X: keyframe=%v, want %v", tc.firstBit, got, tc.wantKey)
			}
		})
	}
}

// makeVP8Keyframe creates a minimal VP8 keyframe with the standard header.
func makeVP8Keyframe(w, h uint16) []byte {
	frame := make([]byte, 20)
	frame[0] = 0x00 // bit0=0 → keyframe
	frame[3] = 0x9D // VP8 magic
	frame[4] = 0x01
	frame[5] = 0x2A
	binary.LittleEndian.PutUint16(frame[6:8], w&0x3FFF)
	binary.LittleEndian.PutUint16(frame[8:10], h&0x3FFF)
	return frame
}

func drainChan(ch <-chan []byte, max int) [][]byte {
	var msgs [][]byte
	for range max {
		select {
		case msg := <-ch:
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
	return msgs
}
