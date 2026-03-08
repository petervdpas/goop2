package modes

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/petervdpas/goop2/internal/call"
	"github.com/petervdpas/goop2/internal/mq"
)

// mqSignalerAdapter bridges *mq.Manager to call.Signaler.
// This is the only place that imports both packages — call knows nothing about mq.
type mqSignalerAdapter struct {
	mqMgr *mq.Manager

	mu    sync.Mutex
	peers map[string]string // channelID → peerID
}

// RegisterChannel associates a call channel ID with the remote peer ID.
// Must be called after StartCall/AcceptCall so Send knows the peer.
func (a *mqSignalerAdapter) RegisterChannel(channelID, peerID string) {
	a.mu.Lock()
	a.peers[channelID] = peerID
	a.mu.Unlock()
}

func (a *mqSignalerAdapter) Send(channelID string, payload any) error {
	a.mu.Lock()
	peerID, ok := a.peers[channelID]
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("mqSignaler: no peer registered for channel %s", channelID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), MQCallSignalTimeout)
	defer cancel()
	_, err := a.mqMgr.Send(ctx, peerID, "call:"+channelID, payload)
	return err
}

func (a *mqSignalerAdapter) PublishLocal(channelID string, payload any) {
	a.mqMgr.PublishLocal("call:"+channelID, "", payload)
}

func (a *mqSignalerAdapter) Subscribe() (chan *call.Envelope, func()) {
	callCh := make(chan *call.Envelope, 64)
	unsub := a.mqMgr.SubscribeTopic("call:", func(from, topic string, payload any) {
		channelID := strings.TrimPrefix(topic, "call:")
		select {
		case callCh <- &call.Envelope{Channel: channelID, From: from, Payload: payload}:
		default:
			log.Printf("mqSignaler: callCh full, dropping envelope for channel %s", channelID)
		}
	})
	cancel := func() {
		unsub()
		close(callCh)
	}
	return callCh, cancel
}
