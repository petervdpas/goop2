package testpeer

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
)

var peerCounter int64

// PeerConfig configures a TestPeer. All fields are optional — sensible
// defaults are generated from a monotonic counter.
type PeerConfig struct {
	ID      string
	Content string
	Email   string
}

// TestPeer is a complete peer in a box: PeerTable + DB + MQ + GroupManager.
// No libp2p, no networking — everything routed through the TestBus.
type TestPeer struct {
	ID      string
	Content string
	Email   string

	Bus     *TestBus
	MQ      *MQAdapter
	Peers   *state.PeerTable
	DB      *storage.DB
	Groups  *group.Manager

	// ResolvePeer is the canonical resolver for this peer.
	// It checks: self → PeerTable → bus peers.
	ResolvePeer func(string) state.PeerIdentityPayload
}

// NewTestPeer creates a fully-wired test peer and registers it on the bus.
// Call t.Cleanup handles teardown — no manual Close needed.
func NewTestPeer(t *testing.T, bus *TestBus, cfg PeerConfig) *TestPeer {
	t.Helper()

	n := atomic.AddInt64(&peerCounter, 1)
	if cfg.ID == "" {
		cfg.ID = fmt.Sprintf("test-peer-%d", n)
	}
	if cfg.Content == "" {
		cfg.Content = fmt.Sprintf("Peer %d", n)
	}
	if cfg.Email == "" {
		cfg.Email = fmt.Sprintf("peer%d@test.local", n)
	}

	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatalf("testpeer: open db: %v", err)
	}

	peers := state.NewPeerTable()
	mqAdapter := NewMQAdapter(bus, cfg.ID)

	tp := &TestPeer{
		ID:      cfg.ID,
		Content: cfg.Content,
		Email:   cfg.Email,
		Bus:     bus,
		MQ:      mqAdapter,
		Peers:   peers,
		DB:      db,
	}

	tp.ResolvePeer = tp.buildResolver()

	tp.Groups = group.NewTestManager(db, cfg.ID, group.TestManagerOpts{
		ResolvePeer: tp.ResolvePeer,
		MQ:          mqAdapter,
	})

	tp.wireIdentityHandlers()

	t.Cleanup(func() {
		tp.Groups.Close()
		mqAdapter.Close()
		db.Close()
	})

	return tp
}

// buildResolver creates a resolvePeer function that checks:
// 1. Self → return own identity
// 2. PeerTable → return cached peer
// 3. Other bus peers → fire MQ identity request (like production)
func (tp *TestPeer) buildResolver() func(string) state.PeerIdentityPayload {
	return func(id string) state.PeerIdentityPayload {
		if id == tp.ID {
			return state.PeerIdentityPayload{
				PeerID:  tp.ID,
				Content: tp.Content,
				Email:   tp.Email,
				Known:   true,
			}
		}
		if sp, ok := tp.Peers.Get(id); ok {
			p := state.FromSeenPeer(sp)
			p.PeerID = id
			return p
		}
		go func() {
			_, _ = tp.MQ.Send(context.Background(), id, mq.TopicIdentity, nil)
		}()
		return state.PeerIdentityPayload{}
	}
}

// wireIdentityHandlers registers the MQ identity request/response handlers,
// exactly mirroring the production code in peer.go.
func (tp *TestPeer) wireIdentityHandlers() {
	tp.MQ.SubscribeTopic(mq.TopicIdentity, func(from, topic string, _ any) {
		if topic != mq.TopicIdentity {
			return
		}
		resp := state.PeerIdentityPayload{
			PeerID:    tp.ID,
			Content:   tp.Content,
			Email:     tp.Email,
			Reachable: true,
		}
		_, _ = tp.MQ.Send(context.Background(), from, mq.TopicIdentityResponse, resp)
	})

	tp.MQ.SubscribeTopic(mq.TopicIdentityResponse, func(from, topic string, payload any) {
		if topic != mq.TopicIdentityResponse {
			return
		}
		pm, ok := payload.(state.PeerIdentityPayload)
		if !ok {
			if m, ok2 := payload.(map[string]any); ok2 {
				content, _ := m["content"].(string)
				email, _ := m["email"].(string)
				if content != "" {
					tp.Peers.Upsert(from, content, email, "", false, "", "", false, false, "")
				}
			}
			return
		}
		if pm.Content != "" {
			tp.Peers.Upsert(from, pm.Content, pm.Email, pm.AvatarHash,
				pm.VideoDisabled, pm.ActiveTemplate, pm.PublicKey,
				pm.EncryptionSupported, pm.Verified, pm.GoopClientVersion)
		}
	})
}

// AnnounceOnline simulates this peer publishing a TypeOnline presence message.
// All other peers on the bus receive the announcement and upsert into their PeerTable.
func (tp *TestPeer) AnnounceOnline() {
	payload := state.PeerIdentityPayload{
		PeerID:    tp.ID,
		Content:   tp.Content,
		Email:     tp.Email,
		Reachable: true,
	}
	tp.Bus.broadcast(tp.ID, "peer:announce", payload)
}

// AnnounceOffline simulates this peer going offline.
func (tp *TestPeer) AnnounceOffline() {
	tp.Bus.broadcast(tp.ID, "peer:gone", mq.PeerGonePayload{PeerID: tp.ID})
}
