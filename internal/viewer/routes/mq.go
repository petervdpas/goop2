package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/petervdpas/goop2/internal/mq"
)

// RegisterMQ adds the message-queue HTTP endpoints.
//
//	POST /api/mq/send   — send a message to a peer
//	POST /api/mq/ack    — notify sender that we processed their message
//	GET  /api/mq/events — SSE stream of incoming messages and delivery receipts
func RegisterMQ(mux *http.ServeMux, mqMgr *mq.Manager) {
	// POST /api/mq/send
	handlePost(mux, "/api/mq/send", func(w http.ResponseWriter, r *http.Request, req struct {
		PeerID  string `json:"peer_id"`
		Topic   string `json:"topic"`
		Payload any    `json:"payload"`
		MsgID   string `json:"msg_id"` // client-generated ID for de-dup (optional)
	}) {
		if req.PeerID == "" || req.Topic == "" {
			http.Error(w, "missing peer_id or topic", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		msgID, err := mqMgr.Send(ctx, req.PeerID, req.Topic, req.Payload)
		if err != nil {
			log.Printf("MQ: send to %s failed: %v", req.PeerID, err)
			http.Error(w, fmt.Sprintf("send failed: %v", err), http.StatusGatewayTimeout)
			return
		}

		writeJSON(w, map[string]string{
			"msg_id": msgID,
			"status": "delivered",
		})
	})

	// POST /api/mq/ack  — browser signals it processed a message; we relay
	// an application-level ACK back to the original sender peer.
	handlePost(mux, "/api/mq/ack", func(w http.ResponseWriter, r *http.Request, req struct {
		MsgID      string `json:"msg_id"`
		FromPeerID string `json:"from_peer_id"`
	}) {
		if req.MsgID == "" {
			http.Error(w, "missing msg_id", http.StatusBadRequest)
			return
		}
		// Empty from_peer_id = local PublishLocal event; no reverse ACK needed.
		if req.FromPeerID == "" {
			mqMgr.NotifyDelivered(req.MsgID)
			writeJSON(w, map[string]string{"status": "ok"})
			return
		}

		// Send an application-level delivery confirmation back to the sender.
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if _, err := mqMgr.Send(ctx, req.FromPeerID, "mq.ack", map[string]string{"msg_id": req.MsgID}); err != nil {
			// Non-fatal: peer may have gone offline. Log and continue.
			log.Printf("MQ: ack relay to %s failed: %v", req.FromPeerID, err)
		}

		// Notify our own SSE listeners that we sent a delivery receipt.
		mqMgr.NotifyDelivered(req.MsgID)

		writeJSON(w, map[string]string{"status": "ok"})
	})

	// GET /api/mq/events — SSE stream
	handleGet(mux, "/api/mq/events", func(w http.ResponseWriter, r *http.Request) {
		sseHeaders(w)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		evtCh, cancel := mqMgr.Subscribe()
		defer cancel()

		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-evtCh:
				if !ok {
					return
				}
				data, err := json.Marshal(evt)
				if err != nil {
					log.Printf("MQ: SSE marshal error: %v", err)
					continue
				}
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	})
}
