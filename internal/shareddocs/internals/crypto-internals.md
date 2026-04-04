# Crypto Internals

## Overview

`internal/crypto/` — NaCl box encryption for peer-to-peer message envelopes. Leaf package with zero imports from other internal packages. Key material is injected via closures.

## Encryptor

```go
type Encryptor struct {
    privKey    [32]byte
    sealKeyFor func(peerID string) (pubKeyB64 string, ok bool)
    openKeyFor func(peerID string) (pubKeyB64 string, ok bool)
}
```

- `sealKeyFor` — used when encrypting: checks both `EncryptionSupported` AND `PublicKey` for the peer
- `openKeyFor` — used when decrypting: only checks if public key exists (always decrypt if key known)
- Two separate lookup functions because a peer may have sent us encrypted data before we knew they support encryption

## Algorithms

- **Key exchange**: X25519 (Curve25519)
- **Encryption**: NaCl box = X25519 + XSalsa20-Poly1305
- **Nonce**: 24 bytes, random per message
- **Wire format**: base64(nonce24 + sealedBox)

## Key management

Keys are generated at first startup and stored in `goop.json`:

- `p2p.nacl_public_key` — base64-encoded 32-byte public key
- `p2p.nacl_private_key` — base64-encoded 32-byte private key

Public keys are announced in `PresenceMsg.publicKey` and exchanged via the PeerTable.

## Error handling

- `ErrNoKey` — returned by `Seal` when the remote peer has no known public key or doesn't support encryption. Callers fall back to plaintext.
- `ErrDecrypt` — returned by `Open` when decryption fails (wrong key, tampered, etc.)

## Usage in goop2

The Encryptor is set on both MQ manager and P2P node:

| Layer | Seal (encrypt) | Open (decrypt) |
| -- | -- | -- |
| MQ messages | `mqMgr.SetEncryptor(enc)` — encrypts MQ payloads per peer | Decrypts inbound MQ payloads |
| Data proxy | `node.SetEncryptor(enc)` — encrypts DataRequest/DataResponse | Lines prefixed with `ENC:` are decrypted before JSON parse |

Encryption is opportunistic: if `Seal` returns `ErrNoKey`, the message is sent in plaintext. This allows mixed networks of encrypted and unencrypted peers.
