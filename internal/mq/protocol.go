// Package mq implements the /goop/mq/1.0.0 message queue transport.
// Wire format: newline-delimited JSON on a persistent libp2p stream.
package mq

// MsgType constants for the wire protocol.
const (
	MsgTypeMsg = "msg" // sender → receiver
	MsgTypeAck = "ack" // receiver → sender (transport ACK)
)

// MQMsg is the wire type for a message sent over the MQ protocol.
type MQMsg struct {
	Type    string `json:"type"`    // "msg"
	ID      string `json:"id"`      // uuid4
	Seq     int64  `json:"seq"`     // monotonic counter per sender
	Topic   string `json:"topic"`   // e.g. "chat", "call:channelID"
	Payload any    `json:"payload"` // arbitrary JSON
}

// MQAck is the wire type for a transport ACK.
type MQAck struct {
	Type string `json:"type"` // "ack"
	ID   string `json:"id"`   // matches MQMsg.ID
	Seq  int64  `json:"seq"`  // matches MQMsg.Seq
}

// mqEvent is delivered to SSE subscribers (/api/mq/events).
type mqEvent struct {
	Type  string `json:"type"`            // "message" | "delivered"
	Msg   *MQMsg `json:"msg,omitempty"`   // set when Type="message"
	MsgID string `json:"msg_id,omitempty"` // set when Type="delivered"
	From  string `json:"from,omitempty"`
}
