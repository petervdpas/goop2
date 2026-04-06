package app

import (
	"net"
	"testing"
	"time"
)

func TestWaitTCP_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if err := WaitTCP(ln.Addr().String(), 2*time.Second); err != nil {
		t.Fatalf("WaitTCP: %v", err)
	}
}

func TestWaitTCP_Timeout(t *testing.T) {
	err := WaitTCP("127.0.0.1:1", 300*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestSetupMicroService_EmptyURL(t *testing.T) {
	called := false
	setupMicroService("Test", "", func() { called = true })
	if called {
		t.Error("configure should not be called for empty URL")
	}
}

func TestSetupMicroService_WithURL(t *testing.T) {
	called := false
	setupMicroService("Test", "http://localhost:8800", func() { called = true })
	if !called {
		t.Error("configure should be called for non-empty URL")
	}
}
