package call

// Signaler is the only surface the call package needs from the realtime layer.
// The concrete mqSignalerAdapter satisfies this via a small adapter in run.go
// (the only place that imports both packages).
type Signaler interface {
	// RegisterChannel tells the signaler which remote peer owns a channel ID.
	// Must be called before Send can route outbound messages for that channel.
	RegisterChannel(channelID, peerID string)
	Send(channelID string, payload any) error
	Subscribe() (ch chan *Envelope, cancel func())
}

// Envelope is a copy of realtime.Envelope — avoids importing internal/realtime.
type Envelope struct {
	Channel string `json:"channel"`
	From    string `json:"from"`
	Payload any    `json:"payload"`
}
