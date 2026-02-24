package call

// Signaler is the only surface the call package needs from the realtime layer.
// The concrete realtime.Manager satisfies this via a small adapter in run.go
// (the only place that imports both packages).
type Signaler interface {
	Send(channelID string, payload any) error
	Subscribe() (ch chan *Envelope, cancel func())
}

// Envelope is a copy of realtime.Envelope — avoids importing internal/realtime.
type Envelope struct {
	Channel string `json:"channel"`
	From    string `json:"from"`
	Payload any    `json:"payload"`
}
