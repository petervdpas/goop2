package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

// genKeyPair returns base64-encoded NaCl keypair.
func genKeyPair(t *testing.T) (pub, priv string) {
	t.Helper()
	pubKey, privKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pubKey[:]),
		base64.StdEncoding.EncodeToString(privKey[:])
}

func TestRoundTrip(t *testing.T) {
	pubA, privA := genKeyPair(t)
	pubB, privB := genKeyPair(t)

	keys := map[string]string{
		"alice": pubA,
		"bob":   pubB,
	}
	lookup := func(peerID string) (string, bool) {
		k, ok := keys[peerID]
		return k, ok
	}

	encA, err := New(privA, lookup)
	if err != nil {
		t.Fatal(err)
	}
	encB, err := New(privB, lookup)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte(`{"type":"call-request","channel":"abc123"}`)

	// Alice seals for Bob
	sealed, err := encA.Seal("bob", plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Bob opens from Alice
	got, err := encB.Open("alice", sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestErrNoKey(t *testing.T) {
	_, privA := genKeyPair(t)

	lookup := func(peerID string) (string, bool) {
		return "", false
	}

	enc, err := New(privA, lookup)
	if err != nil {
		t.Fatal(err)
	}

	_, err = enc.Seal("unknown-peer", []byte("hello"))
	if !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
}

func TestCrossPeerDecryptFails(t *testing.T) {
	pubA, privA := genKeyPair(t)
	_, privC := genKeyPair(t)
	pubB, _ := genKeyPair(t)

	keysA := map[string]string{"bob": pubB}
	encA, _ := New(privA, func(id string) (string, bool) {
		k, ok := keysA[id]
		return k, ok
	})

	// C tries to decrypt a message from A to B (C doesn't have B's private key)
	keysC := map[string]string{"alice": pubA}
	encC, _ := New(privC, func(id string) (string, bool) {
		k, ok := keysC[id]
		return k, ok
	})

	sealed, err := encA.Seal("bob", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = encC.Open("alice", sealed)
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("expected ErrDecrypt, got %v", err)
	}
}

func TestEmptyPlaintext(t *testing.T) {
	pubA, privA := genKeyPair(t)
	pubB, privB := genKeyPair(t)

	keys := map[string]string{"alice": pubA, "bob": pubB}
	lookup := func(id string) (string, bool) {
		k, ok := keys[id]
		return k, ok
	}

	encA, _ := New(privA, lookup)
	encB, _ := New(privB, lookup)

	sealed, err := encA.Seal("bob", []byte{})
	if err != nil {
		t.Fatal(err)
	}

	got, err := encB.Open("alice", sealed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes", len(got))
	}
}

func TestTamperedCiphertext(t *testing.T) {
	pubA, privA := genKeyPair(t)
	pubB, privB := genKeyPair(t)

	keys := map[string]string{"alice": pubA, "bob": pubB}
	lookup := func(id string) (string, bool) {
		k, ok := keys[id]
		return k, ok
	}

	encA, _ := New(privA, lookup)
	encB, _ := New(privB, lookup)

	sealed, _ := encA.Seal("bob", []byte("hello"))

	// Tamper with the ciphertext
	raw, _ := base64.StdEncoding.DecodeString(sealed)
	raw[len(raw)-1] ^= 0xff
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err := encB.Open("alice", tampered)
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("expected ErrDecrypt for tampered data, got %v", err)
	}
}
