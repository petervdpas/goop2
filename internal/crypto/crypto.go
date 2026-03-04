// Package crypto provides NaCl box encryption for peer-to-peer message envelopes.
// It is a leaf package — zero imports from other internal packages. Key material
// is injected via closures so it stays decoupled from peer state.
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/box"
)

var (
	// ErrNoKey is returned by Seal when the remote peer has no known public key.
	// Callers should fall back to plaintext when they receive this error.
	ErrNoKey = errors.New("crypto: no public key for peer")

	// ErrDecrypt is returned by Open when decryption fails (wrong key, tampered, etc).
	ErrDecrypt = errors.New("crypto: decryption failed")
)

const nonceSize = 24

// Encryptor seals and opens NaCl box messages between peers.
type Encryptor struct {
	privKey      [32]byte
	sealKeyFor   func(peerID string) (string, bool) // Seal: checks EncryptionSupported + PublicKey
	openKeyFor   func(peerID string) (string, bool) // Open: only checks PublicKey (always decrypt if key known)
}

// New creates an Encryptor from a base64-encoded private key and two lookup
// functions: sealKeyFor (for encrypting — should check EncryptionSupported)
// and openKeyFor (for decrypting — should only check if public key exists).
func New(privKeyB64 string, sealKeyFor, openKeyFor func(peerID string) (string, bool)) (*Encryptor, error) {
	raw, err := base64.StdEncoding.DecodeString(privKeyB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode private key: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("crypto: private key must be 32 bytes, got %d", len(raw))
	}
	e := &Encryptor{sealKeyFor: sealKeyFor, openKeyFor: openKeyFor}
	copy(e.privKey[:], raw)
	return e, nil
}

// Seal encrypts plaintext for the given peer. Returns a base64 string
// containing nonce24 + sealedBox, or ErrNoKey if the peer has no public key
// or doesn't support encryption.
func (e *Encryptor) Seal(peerID string, plaintext []byte) (string, error) {
	pubB64, ok := e.sealKeyFor(peerID)
	if !ok || pubB64 == "" {
		return "", ErrNoKey
	}

	pubRaw, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return "", fmt.Errorf("crypto: decode peer public key: %w", err)
	}
	if len(pubRaw) != 32 {
		return "", fmt.Errorf("crypto: peer public key must be 32 bytes, got %d", len(pubRaw))
	}

	var peerPub [32]byte
	copy(peerPub[:], pubRaw)

	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	sealed := box.Seal(nonce[:], plaintext, &nonce, &peerPub, &e.privKey)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a base64 string (nonce24 + sealedBox) from the given peer.
// Uses openKeyFor which only requires the peer's public key (no EncryptionSupported check).
func (e *Encryptor) Open(peerID string, ciphertextB64 string) ([]byte, error) {
	pubB64, ok := e.openKeyFor(peerID)
	if !ok || pubB64 == "" {
		return nil, ErrNoKey
	}

	pubRaw, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode peer public key: %w", err)
	}
	if len(pubRaw) != 32 {
		return nil, fmt.Errorf("crypto: peer public key must be 32 bytes, got %d", len(pubRaw))
	}

	raw, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode ciphertext: %w", err)
	}
	if len(raw) < nonceSize+box.Overhead {
		return nil, ErrDecrypt
	}

	var peerPub [32]byte
	copy(peerPub[:], pubRaw)

	var nonce [nonceSize]byte
	copy(nonce[:], raw[:nonceSize])

	plaintext, ok := box.Open(nil, raw[nonceSize:], &nonce, &peerPub, &e.privKey)
	if !ok {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
