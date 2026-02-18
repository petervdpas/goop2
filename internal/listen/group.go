// Package listen implements a listening group — a host streams audio in
// real-time to connected listeners via the group protocol (control) and
// a dedicated binary stream protocol (audio data).
package listen

// Group represents an active listening group.
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"` // "host" or "listener"

	Track     *Track     `json:"track,omitempty"`
	PlayState *PlayState `json:"play_state,omitempty"`
	Listeners []string   `json:"listeners,omitempty"` // peer IDs

	// Queue info — track names visible to all participants.
	Queue      []string `json:"queue,omitempty"` // display names of all queued tracks
	QueueIndex int      `json:"queue_index"`     // 0-based index of current track
	QueueTotal int      `json:"queue_total"`     // total tracks in queue (0 = no queue)
}

// Track describes the currently loaded audio track.
type Track struct {
	Name     string  `json:"name"`
	Duration float64 `json:"duration"` // seconds
	Bitrate  int     `json:"bitrate"`  // bits per second
	Format   string  `json:"format"`   // "mp3"
}

// PlayState describes the current playback position.
type PlayState struct {
	Playing   bool    `json:"playing"`
	Position  float64 `json:"position"`   // seconds
	UpdatedAt int64   `json:"updated_at"` // unix millis
}

// ControlMsg is the envelope sent over the group protocol for listen events.
type ControlMsg struct {
	Action     string   `json:"action"`              // load, play, pause, seek, sync, close
	Track      *Track   `json:"track,omitempty"`     // set on "load"
	Position   float64  `json:"position,omitempty"`  // set on "seek", "sync", "play"
	Queue      []string `json:"queue,omitempty"`     // track names; set on "load"
	QueueIndex int      `json:"queue_index"`         // current track index; set on "load"
	QueueTotal int      `json:"queue_total"`         // total tracks; set on "load"
}
