package p2p

import (
	"context"
	"io"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	circuitv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
)

const testProto = protocol.ID("/test/1.0.0")

func setupRelay(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })

	_, err = relayv2.New(h)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func setupPeer(t *testing.T, relay host.Host) host.Host {
	t.Helper()
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.EnableRelay(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })

	relayInfo := peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}
	if err := h.Connect(context.Background(), relayInfo); err != nil {
		t.Fatal(err)
	}

	_, err = circuitv2.Reserve(context.Background(), h, relayInfo)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestDirectLAN_StreamSucceeds(t *testing.T) {
	peerA, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer peerA.Close()

	peerB, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer peerB.Close()

	peerB.SetStreamHandler(testProto, func(s network.Stream) {
		s.Write([]byte("lan-ok"))
		s.Close()
	})

	if err := peerA.Connect(context.Background(), peer.AddrInfo{ID: peerB.ID(), Addrs: peerB.Addrs()}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s, err := peerA.NewStream(ctx, peerB.ID(), testProto)
	if err != nil {
		t.Fatalf("LAN direct stream should succeed: %v", err)
	}
	defer s.Close()

	buf, _ := io.ReadAll(s)
	if string(buf) != "lan-ok" {
		t.Fatalf("expected 'lan-ok', got %q", string(buf))
	}
}

func TestDirectLAN_StreamSucceeds_WithAllowLimited(t *testing.T) {
	peerA, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer peerA.Close()

	peerB, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer peerB.Close()

	peerB.SetStreamHandler(testProto, func(s network.Stream) {
		s.Write([]byte("lan-ok"))
		s.Close()
	})

	if err := peerA.Connect(context.Background(), peer.AddrInfo{ID: peerB.ID(), Addrs: peerB.Addrs()}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "relay")

	s, err := peerA.NewStream(ctx, peerB.ID(), testProto)
	if err != nil {
		t.Fatalf("LAN direct stream with AllowLimited should still succeed: %v", err)
	}
	defer s.Close()

	buf, _ := io.ReadAll(s)
	if string(buf) != "lan-ok" {
		t.Fatalf("expected 'lan-ok', got %q", string(buf))
	}
}

func TestCircuitWAN_FailsWithoutAllowLimitedConn(t *testing.T) {
	relay := setupRelay(t)
	peerA := setupPeer(t, relay)
	peerB := setupPeer(t, relay)

	peerB.SetStreamHandler(testProto, func(s network.Stream) {
		s.Write([]byte("hello"))
		s.Close()
	})

	circuitAddr, _ := ma.NewMultiaddr(
		"/p2p/" + relay.ID().String() + "/p2p-circuit/p2p/" + peerB.ID().String(),
	)
	peerA.Peerstore().AddAddrs(peerB.ID(), []ma.Multiaddr{circuitAddr}, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := peerA.NewStream(ctx, peerB.ID(), testProto)
	if err == nil {
		t.Fatal("circuit stream should fail WITHOUT WithAllowLimitedConn")
	}
}

func TestCircuitWAN_SucceedsWithAllowLimitedConn(t *testing.T) {
	relay := setupRelay(t)
	peerA := setupPeer(t, relay)
	peerB := setupPeer(t, relay)

	peerB.SetStreamHandler(testProto, func(s network.Stream) {
		s.Write([]byte("wan-ok"))
		s.Close()
	})

	circuitAddr, _ := ma.NewMultiaddr(
		"/p2p/" + relay.ID().String() + "/p2p-circuit/p2p/" + peerB.ID().String(),
	)
	peerA.Peerstore().AddAddrs(peerB.ID(), []ma.Multiaddr{circuitAddr}, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "relay")

	s, err := peerA.NewStream(ctx, peerB.ID(), testProto)
	if err != nil {
		t.Fatalf("circuit stream should succeed WITH WithAllowLimitedConn: %v", err)
	}
	defer s.Close()

	buf, _ := io.ReadAll(s)
	if string(buf) != "wan-ok" {
		t.Fatalf("expected 'wan-ok', got %q", string(buf))
	}
}

func TestCircuitWAN_BidirectionalStream(t *testing.T) {
	relay := setupRelay(t)
	peerA := setupPeer(t, relay)
	peerB := setupPeer(t, relay)

	peerB.SetStreamHandler(testProto, func(s network.Stream) {
		buf := make([]byte, 64)
		n, _ := s.Read(buf)
		s.Write([]byte("reply:" + string(buf[:n])))
		s.Close()
	})

	circuitAddr, _ := ma.NewMultiaddr(
		"/p2p/" + relay.ID().String() + "/p2p-circuit/p2p/" + peerB.ID().String(),
	)
	peerA.Peerstore().AddAddrs(peerB.ID(), []ma.Multiaddr{circuitAddr}, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "relay")

	s, err := peerA.NewStream(ctx, peerB.ID(), testProto)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.Write([]byte("ping"))
	s.CloseWrite()

	buf, _ := io.ReadAll(s)
	if string(buf) != "reply:ping" {
		t.Fatalf("expected 'reply:ping', got %q", string(buf))
	}
}

func TestMixedTopology_LANPeerAndCircuitPeer(t *testing.T) {
	relay := setupRelay(t)

	lanPeer, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer lanPeer.Close()

	wanPeer := setupPeer(t, relay)

	lanPeer.SetStreamHandler(testProto, func(s network.Stream) {
		s.Write([]byte("from-lan"))
		s.Close()
	})
	wanPeer.SetStreamHandler(testProto, func(s network.Stream) {
		s.Write([]byte("from-wan"))
		s.Close()
	})

	hub, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.EnableRelay(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()

	if err := hub.Connect(context.Background(), peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}); err != nil {
		t.Fatal(err)
	}
	if _, err := circuitv2.Reserve(context.Background(), hub, peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}); err != nil {
		t.Fatal(err)
	}

	if err := hub.Connect(context.Background(), peer.AddrInfo{ID: lanPeer.ID(), Addrs: lanPeer.Addrs()}); err != nil {
		t.Fatal(err)
	}

	circuitAddr, _ := ma.NewMultiaddr(
		"/p2p/" + relay.ID().String() + "/p2p-circuit/p2p/" + wanPeer.ID().String(),
	)
	hub.Peerstore().AddAddrs(wanPeer.ID(), []ma.Multiaddr{circuitAddr}, time.Hour)

	ctx := network.WithAllowLimitedConn(context.Background(), "relay")

	ctxLAN, cancelLAN := context.WithTimeout(ctx, 5*time.Second)
	defer cancelLAN()
	sLAN, err := hub.NewStream(ctxLAN, lanPeer.ID(), testProto)
	if err != nil {
		t.Fatalf("LAN stream failed: %v", err)
	}
	bufLAN, _ := io.ReadAll(sLAN)
	sLAN.Close()
	if string(bufLAN) != "from-lan" {
		t.Fatalf("expected 'from-lan', got %q", string(bufLAN))
	}

	ctxWAN, cancelWAN := context.WithTimeout(ctx, 5*time.Second)
	defer cancelWAN()
	sWAN, err := hub.NewStream(ctxWAN, wanPeer.ID(), testProto)
	if err != nil {
		t.Fatalf("WAN circuit stream failed: %v", err)
	}
	bufWAN, _ := io.ReadAll(sWAN)
	sWAN.Close()
	if string(bufWAN) != "from-wan" {
		t.Fatalf("expected 'from-wan', got %q", string(bufWAN))
	}
}
