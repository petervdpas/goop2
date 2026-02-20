package listen

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// mp3Info holds bitrate and duration extracted from an MP3 file header.
type mp3Info struct {
	Bitrate  int     // bits per second
	Duration float64 // seconds
}

// MPEG audio version/layer/bitrate lookup tables (ISO 11172-3 / 13818-3).
var bitrateTable = [2][3][16]int{
	// MPEG-1
	{
		// Layer I
		{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
		// Layer II
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
		// Layer III
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	},
	// MPEG-2 / MPEG-2.5
	{
		// Layer I
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
		// Layer II
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		// Layer III
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
	},
}

var sampleRateTable = [3][4]int{
	{44100, 48000, 32000, 0}, // MPEG-1
	{22050, 24000, 16000, 0}, // MPEG-2
	{11025, 12000, 8000, 0},  // MPEG-2.5
}

// probeMP3 reads the first few KB of an MP3 file to determine bitrate,
// then uses file size to estimate duration.
func probeMP3(path string) (*mp3Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := stat.Size()

	// Skip ID3v2 tag if present
	var header [10]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	offset := int64(0)
	if string(header[:3]) == "ID3" {
		// Synchsafe integer (4 bytes, 7 bits each)
		tagSize := int64(header[6])<<21 | int64(header[7])<<14 | int64(header[8])<<7 | int64(header[9])
		offset = 10 + tagSize
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	// Scan for the first valid MPEG frame sync (0xFF 0xE0+)
	buf := make([]byte, 8192)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	buf = buf[:n]

	for i := 0; i < len(buf)-4; i++ {
		if buf[i] != 0xFF || buf[i+1]&0xE0 != 0xE0 {
			continue
		}

		hdr := binary.BigEndian.Uint32(buf[i : i+4])

		// Parse frame header fields
		versionBits := (hdr >> 19) & 0x03
		layerBits := (hdr >> 17) & 0x03
		bitrateIdx := (hdr >> 12) & 0x0F
		sampleIdx := (hdr >> 10) & 0x03

		if bitrateIdx == 0 || bitrateIdx == 15 || sampleIdx == 3 {
			continue
		}
		if layerBits == 0 { // reserved
			continue
		}

		// Map version bits: 0=2.5, 1=reserved, 2=2, 3=1
		versionIdx := 0 // MPEG-2/2.5 for bitrate table
		sampleVersion := 0
		switch versionBits {
		case 3:
			versionIdx = 0 // MPEG-1
			sampleVersion = 0
		case 2:
			versionIdx = 1 // MPEG-2
			sampleVersion = 1
		case 0:
			versionIdx = 1 // MPEG-2.5
			sampleVersion = 2
		default:
			continue // reserved
		}

		// Map layer bits: 1=III, 2=II, 3=I
		layerIdx := 0
		switch layerBits {
		case 3:
			layerIdx = 0 // Layer I
		case 2:
			layerIdx = 1 // Layer II
		case 1:
			layerIdx = 2 // Layer III
		default:
			continue
		}

		bitrate := bitrateTable[versionIdx][layerIdx][bitrateIdx] * 1000
		sampleRate := sampleRateTable[sampleVersion][sampleIdx]

		if bitrate == 0 || sampleRate == 0 {
			continue
		}

		// Estimate duration from file size and bitrate
		audioSize := fileSize - offset
		duration := float64(audioSize*8) / float64(bitrate)

		return &mp3Info{
			Bitrate:  bitrate,
			Duration: duration,
		}, nil
	}

	return nil, fmt.Errorf("no valid MPEG frame found")
}

// ratePacer writes MP3 data to w without rate limiting.
// The browser's audio buffer provides natural rate control via TCP flow control.
// It reads from the file and stops when done, the context is cancelled (via the done channel), or an error occurs.
type ratePacer struct {
	file    *os.File
	bitrate int // bits per second (used for display only, not for pacing)
	done    <-chan struct{}
}

// stream writes audio bytes to w as fast as the network allows.
// Returns nil on EOF (track finished), or an error.
func (rp *ratePacer) stream(w io.Writer) error {
	// Use a larger buffer (64KB) so data flows smoothly to the listener.
	// TCP/browser buffering naturally rate-limits this.
	buf := make([]byte, 64*1024)

	for {
		// Non-blocking check if playback should stop
		select {
		case <-rp.done:
			return nil
		default:
		}

		// Read and stream audio data
		n, err := rp.file.Read(buf)
		if n > 0 {
			data := buf[:n]
			for len(data) > 0 {
				nw, werr := w.Write(data)
				if werr != nil {
					return werr
				}
				data = data[nw:]
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
