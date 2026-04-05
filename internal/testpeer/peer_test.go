package testpeer

import (
	"testing"
	"time"

	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/state"
)

func TestBusRouting_DirectMessage(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice"})
	bob := NewTestPeer(t, bus, PeerConfig{Content: "Bob"})

	var received string
	bob.MQ.SubscribeTopic("chat", func(from, topic string, payload any) {
		received = from
	})

	msgID, err := alice.MQ.Send(nil, bob.ID, "chat", "hello")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if msgID == "" {
		t.Fatal("expected non-empty message ID")
	}
	if received != alice.ID {
		t.Fatalf("expected from=%q, got %q", alice.ID, received)
	}
}

func TestBusRouting_TopicPrefixMatch(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice"})
	bob := NewTestPeer(t, bus, PeerConfig{Content: "Bob"})

	var topics []string
	bob.MQ.SubscribeTopic("group:", func(from, topic string, payload any) {
		topics = append(topics, topic)
	})

	alice.MQ.Send(nil, bob.ID, "group:g1:join", nil)
	alice.MQ.Send(nil, bob.ID, "group:g1:leave", nil)
	alice.MQ.Send(nil, bob.ID, "chat", nil)

	if len(topics) != 2 {
		t.Fatalf("expected 2 group: matches, got %d: %v", len(topics), topics)
	}
}

func TestBusRouting_UnknownPeerReturnsError(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice"})

	_, err := alice.MQ.Send(nil, "nonexistent-peer", "chat", nil)
	if err == nil {
		t.Fatal("expected error for unknown peer")
	}
}

func TestBusMessageLog(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice"})
	bob := NewTestPeer(t, bus, PeerConfig{Content: "Bob"})

	alice.MQ.Send(nil, bob.ID, "chat", "hi")
	bob.MQ.Send(nil, alice.ID, "chat", "hey")

	if bus.MessageCount() != 2 {
		t.Fatalf("expected 2 messages, got %d", bus.MessageCount())
	}

	chatMsgs := bus.MessagesForTopic("chat")
	if len(chatMsgs) != 2 {
		t.Fatalf("expected 2 chat messages, got %d", len(chatMsgs))
	}
}

func TestIdentityResolution_ViaMQFallback(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice"})
	bob := NewTestPeer(t, bus, PeerConfig{Content: "Bob"})

	identity := alice.ResolvePeer(bob.ID)
	if identity.Known {
		t.Fatal("Bob should be unknown initially — not in Alice's PeerTable")
	}

	// The resolver fires the identity request in a goroutine (like production).
	// Wait briefly for the async MQ round-trip to complete.
	for range 50 {
		if _, ok := alice.Peers.Get(bob.ID); ok {
			break
		}
		time.Sleep(time.Millisecond)
	}

	identityMsgs := bus.MessagesForTopic(mq.TopicIdentity)
	if len(identityMsgs) == 0 {
		t.Fatal("expected identity request to be sent")
	}

	responseMsgs := bus.MessagesForTopic(mq.TopicIdentityResponse)
	if len(responseMsgs) == 0 {
		t.Fatal("expected identity response")
	}

	sp, ok := alice.Peers.Get(bob.ID)
	if !ok {
		t.Fatal("Bob should now be in Alice's PeerTable after identity exchange")
	}
	if sp.Content != "Bob" {
		t.Fatalf("expected Content='Bob', got %q", sp.Content)
	}
}

func TestIdentityResolution_SelfIsAlwaysKnown(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice", Email: "alice@test.local"})

	identity := alice.ResolvePeer(alice.ID)
	if !identity.Known {
		t.Fatal("self should always be known")
	}
	if identity.Name() != "Alice" {
		t.Fatalf("expected Name='Alice', got %q", identity.Name())
	}
	if identity.Email != "alice@test.local" {
		t.Fatalf("expected Email='alice@test.local', got %q", identity.Email)
	}
}

func TestPresenceAnnounce_BroadcastsToAllPeers(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice"})
	bob := NewTestPeer(t, bus, PeerConfig{Content: "Bob"})
	charlie := NewTestPeer(t, bus, PeerConfig{Content: "Charlie"})

	var bobGotAnnounce, charlieGotAnnounce bool
	bob.MQ.SubscribeTopic("peer:announce", func(from, topic string, payload any) {
		if p, ok := payload.(state.PeerIdentityPayload); ok && p.Content == "Alice" {
			bobGotAnnounce = true
		}
	})
	charlie.MQ.SubscribeTopic("peer:announce", func(from, topic string, payload any) {
		if p, ok := payload.(state.PeerIdentityPayload); ok && p.Content == "Alice" {
			charlieGotAnnounce = true
		}
	})

	alice.AnnounceOnline()

	if !bobGotAnnounce {
		t.Fatal("Bob should have received Alice's announce")
	}
	if !charlieGotAnnounce {
		t.Fatal("Charlie should have received Alice's announce")
	}

	_ = alice // Alice should NOT receive her own announce
	aliceAnnounces := bus.Messages(func(m BusMessage) bool {
		return m.Topic == "peer:announce" && m.To == alice.ID
	})
	if len(aliceAnnounces) != 0 {
		t.Fatal("Alice should not receive her own broadcast")
	}
}

func TestGroupLifecycle_HostAndRemotePeer(t *testing.T) {
	bus := NewTestBus()
	host := NewTestPeer(t, bus, PeerConfig{ID: "host-peer", Content: "Host"})

	err := host.Groups.CreateGroup("g1", "Test Room", "template", "blog", 0)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	err = host.Groups.JoinOwnGroup("g1")
	if err != nil {
		t.Fatalf("host join failed: %v", err)
	}

	members := host.Groups.HostedGroupMembers("g1")
	if len(members) != 1 {
		t.Fatalf("expected 1 member (host), got %d", len(members))
	}

	host.Groups.SimulateJoin("remote-peer", "g1")

	members = host.Groups.HostedGroupMembers("g1")
	if len(members) != 2 {
		t.Fatalf("expected 2 members after join, got %d", len(members))
	}

	welcomeMsgs := bus.MessagesForTopic("group:g1:" + "welcome")
	if len(welcomeMsgs) == 0 {
		t.Fatal("expected welcome message to be sent to remote peer")
	}
	if welcomeMsgs[0].To != "remote-peer" {
		t.Fatalf("welcome should go to remote-peer, got %q", welcomeMsgs[0].To)
	}

	host.Groups.SimulateLeave("remote-peer", "g1")

	members = host.Groups.HostedGroupMembers("g1")
	if len(members) != 1 {
		t.Fatalf("expected 1 member after leave, got %d", len(members))
	}
}

func TestPublishLocal_DeliveredToOwnSubscribers(t *testing.T) {
	bus := NewTestBus()
	alice := NewTestPeer(t, bus, PeerConfig{Content: "Alice"})

	var received bool
	alice.MQ.SubscribeTopic("local:event", func(from, topic string, payload any) {
		received = true
	})

	alice.MQ.PublishLocal("local:event", "", "test-payload")

	if !received {
		t.Fatal("PublishLocal should deliver to own subscribers")
	}
}

func TestThreePeerMessageChain(t *testing.T) {
	bus := NewTestBus()
	a := NewTestPeer(t, bus, PeerConfig{Content: "A"})
	b := NewTestPeer(t, bus, PeerConfig{Content: "B"})
	c := NewTestPeer(t, bus, PeerConfig{Content: "C"})

	b.MQ.SubscribeTopic("relay:", func(from, topic string, payload any) {
		msg, _ := payload.(string)
		b.MQ.Send(nil, c.ID, "relay:forwarded", msg+" via B")
	})

	var finalMsg string
	c.MQ.SubscribeTopic("relay:", func(from, topic string, payload any) {
		finalMsg, _ = payload.(string)
	})

	a.MQ.Send(nil, b.ID, "relay:start", "hello from A")

	if finalMsg != "hello from A via B" {
		t.Fatalf("expected forwarded message, got %q", finalMsg)
	}

	if bus.MessageCount() != 2 {
		t.Fatalf("expected 2 messages (A→B, B→C), got %d", bus.MessageCount())
	}
}
