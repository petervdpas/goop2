package p2p

import (
	"io"
	"testing"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	ymux "github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	yamux "github.com/libp2p/go-yamux/v4"
)

func TestNodeLibp2pOptions_NoConflict(t *testing.T) {
	ymuxCfg := yamux.DefaultConfig()
	ymuxCfg.KeepAliveInterval = YamuxKeepAlive
	ymuxCfg.LogOutput = io.Discard

	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Muxer(ymux.ID, (*ymux.Transport)(ymuxCfg)),
		libp2p.DefaultTransports,
	)
	if err != nil {
		t.Fatalf("libp2p.New with production options failed: %v", err)
	}
	defer h.Close()

	if h.ID() == "" {
		t.Fatal("expected valid peer ID")
	}
}

func TestNodeLibp2pOptions_CanDialWSS(t *testing.T) {
	ymuxCfg := yamux.DefaultConfig()
	ymuxCfg.KeepAliveInterval = YamuxKeepAlive
	ymuxCfg.LogOutput = io.Discard

	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Muxer(ymux.ID, (*ymux.Transport)(ymuxCfg)),
		libp2p.DefaultTransports,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	protos := h.Mux().Protocols()
	if len(protos) == 0 {
		t.Fatal("expected registered protocols")
	}
}

func TestWithAllowLimitedConn_ContextHasFlag(t *testing.T) {
	ctx := t.Context()
	allowed, _ := network.GetAllowLimitedConn(ctx)
	if allowed {
		t.Fatal("plain context should not allow limited conn")
	}

	ctx = network.WithAllowLimitedConn(ctx, "relay")
	allowed, reason := network.GetAllowLimitedConn(ctx)
	if !allowed {
		t.Fatal("context with WithAllowLimitedConn should allow limited conn")
	}
	if reason != "relay" {
		t.Fatalf("expected reason 'relay', got %q", reason)
	}
}
