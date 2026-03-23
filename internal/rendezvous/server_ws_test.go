package rendezvous

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/proto"
)

func publishPeer(t *testing.T, baseURL, peerID string) {
	t.Helper()
	msg := proto.PresenceMsg{
		Type:   proto.TypeOnline,
		PeerID: peerID,
		Content: peerID,
		TS:     proto.NowMillis(),
	}
	b, _ := json.Marshal(msg)
	resp, err := http.Post(baseURL+"/publish", "application/json", strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("publish %s: %v", peerID, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("publish %s: status %d", peerID, resp.StatusCode)
	}
}

func dialWS(t *testing.T, baseURL, peerID string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws?peer_id=" + peerID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial WS %s: %v", peerID, err)
	}
	return conn
}

func tryDialWS(baseURL, peerID string) (*websocket.Conn, error) {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws?peer_id=" + peerID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	return conn, err
}

func TestWSRaceConditions(t *testing.T) {
	srv := New("127.0.0.1:18791", "", "", "", 0, 0, "", RelayTimingConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	base := srv.URL()

	clearRate := func() {
		srv.mu.Lock()
		srv.rateWindow = map[string]*rateBucket{}
		srv.mu.Unlock()
	}

	t.Run("BroadcastDuringDisconnect", func(t *testing.T) {
		const numPeers = 10
		for i := range numPeers {
			publishPeer(t, base, fmt.Sprintf("bd-%d", i))
		}

		conns := make([]*websocket.Conn, numPeers)
		for i := range numPeers {
			conns[i] = dialWS(t, base, fmt.Sprintf("bd-%d", i))
		}
		time.Sleep(50 * time.Millisecond)

		// Concurrently: close half the connections while the other half
		// send presence updates (triggering broadcasts). Previously this
		// would panic with "send on closed channel".
		var wg sync.WaitGroup
		for i := range numPeers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if i%2 == 0 {
					conns[i].Close()
				} else {
					msg := proto.PresenceMsg{
						Type:    proto.TypeUpdate,
						PeerID:  fmt.Sprintf("bd-%d", i),
						Content: fmt.Sprintf("bd-%d", i),
						TS:      proto.NowMillis(),
					}
					b, _ := json.Marshal(msg)
					for range 20 {
						conns[i].WriteMessage(websocket.TextMessage, b)
						time.Sleep(time.Millisecond)
					}
					conns[i].Close()
				}
			}()
		}
		wg.Wait()
	})

	clearRate()
	t.Run("ReconnectReplacesSafely", func(t *testing.T) {
		connected := 0
		for range 20 {
			publishPeer(t, base, "rc-peer")
			conn, err := tryDialWS(base, "rc-peer")
			if err != nil {
				continue // previous disconnect may have raced with publish
			}
			connected++
			msg := proto.PresenceMsg{
				Type:    proto.TypeUpdate,
				PeerID:  "rc-peer",
				Content: "rc-peer",
				TS:      proto.NowMillis(),
			}
			b, _ := json.Marshal(msg)
			conn.WriteMessage(websocket.TextMessage, b)
		}
		if connected < 5 {
			t.Fatalf("too few successful connections: %d/20", connected)
		}
		time.Sleep(100 * time.Millisecond)
	})

	clearRate()
	t.Run("ConcurrentBroadcastAndReconnect", func(t *testing.T) {
		const numPeers = 5
		for i := range numPeers {
			publishPeer(t, base, fmt.Sprintf("cbr-%d", i))
		}

		var wg sync.WaitGroup
		for i := range numPeers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				peerID := fmt.Sprintf("cbr-%d", i)
				for range 10 {
					publishPeer(t, base, peerID)
					conn, err := tryDialWS(base, peerID)
					if err != nil {
						continue
					}
					msg := proto.PresenceMsg{
						Type:    proto.TypeUpdate,
						PeerID:  peerID,
						Content: peerID,
						TS:      proto.NowMillis(),
					}
					b, _ := json.Marshal(msg)
					conn.WriteMessage(websocket.TextMessage, b)
					time.Sleep(2 * time.Millisecond)
					conn.Close()
				}
			}()
		}
		wg.Wait()
	})

	clearRate()
	t.Run("SendToPeerAfterDisconnect", func(t *testing.T) {
		publishPeer(t, base, "stpad-sender")
		publishPeer(t, base, "stpad-receiver")

		senderConn := dialWS(t, base, "stpad-sender")
		receiverConn := dialWS(t, base, "stpad-receiver")
		time.Sleep(50 * time.Millisecond)

		receiverConn.Close()
		time.Sleep(50 * time.Millisecond)

		msg := proto.PresenceMsg{
			Type:    proto.TypeUpdate,
			PeerID:  "stpad-sender",
			Content: "stpad-sender",
			TS:      proto.NowMillis(),
		}
		b, _ := json.Marshal(msg)
		for range 10 {
			senderConn.WriteMessage(websocket.TextMessage, b)
			time.Sleep(5 * time.Millisecond)
		}
		senderConn.Close()
	})

	t.Run("ProbeDoesNotPanic", func(t *testing.T) {
		wsURL := "ws" + strings.TrimPrefix(base, "http") + "/ws?probe=1"
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("probe dial: %v", err)
		}
		conn.ReadMessage()
		conn.Close()
	})

	t.Run("UnknownPeerRejected", func(t *testing.T) {
		wsURL := "ws" + strings.TrimPrefix(base, "http") + "/ws?peer_id=unknown-peer"
		_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err == nil {
			t.Fatal("expected dial to fail for unknown peer")
		}
		if resp != nil && resp.StatusCode != http.StatusTooEarly {
			t.Fatalf("expected 425, got %d", resp.StatusCode)
		}
	})
}
