# MQ Internals

## Protocol

Protocol ID: `/goop/mq/1.0.0` (`internal/mq/protocol.go`)

Wire format: newline-delimited JSON on a persistent libp2p stream.

### Message types

**MQMsg** (sender → receiver):
```
{"type":"msg", "id":"<uuid4>", "seq":<int64>, "topic":"<string>", "payload":<any>}
```

**MQAck** (receiver → sender):
```
{"type":"ack", "id":"<matches msg id>", "seq":<matches msg seq>}
```

ACK timeout: 4 seconds (`internal/mq/constants.go`). Messages not ACKed within this window are considered failed.

### Encryption

When an MQEncryptor is set, message payloads are NaCl-encrypted per peer. If `Seal` returns `ErrNoKey`, the message falls back to plaintext.

## Topics

All topic constants are in `internal/mq/topics.go`. Mirrored in `internal/ui/assets/js/mq/topics.js` — must be kept in sync.

| Topic | Direction | Purpose |
| -- | -- | -- |
| `peer:announce` | local | Peer came online or updated (PeerTable → browser SSE) |
| `peer:gone` | local | Peer pruned from PeerTable |
| `chat` | P2P | Direct chat message to a specific peer |
| `chat.broadcast` | P2P | Broadcast chat to all peers |
| `chat.room:{groupID}:{sub}` | P2P + local | Group chat room events. Sub: `msg`, `history`, `members` |
| `group:{groupID}:{type}` | P2P | Group protocol messages: `join`, `welcome`, `members`, `msg`, `state`, `leave`, `close`, `error`, `ping`, `pong`, `meta` |
| `group.invite` | P2P | Group invitation delivery |
| `call:{channelID}` | P2P | Call signaling: `call-request`, `call-ack`, `call-offer`, `call-answer`, `ice-candidate`, `call-hangup` |
| `call:loopback:{channelID}` | local | Go → browser ICE candidates for native WebRTC (Phase 4) |
| `listen:{groupID}:state` | local | Listen player state updates |
| `identity` | P2P | Request a peer's full identity (timing race fallback) |
| `identity.response` | P2P | Full identity reply: name, email, avatar, version, etc. |
| `log:mq` | local | Internal MQ event log |

## SSE endpoints

Only two SSE endpoints exist in the system:

| Endpoint | Purpose |
| -- | -- |
| `/api/mq/events` | MQ bus — all real-time events delivered to the browser |
| `/api/logs/stream` | Log tailing — streams log buffer to the browser |

## Local vs remote

| Method | Delivery |
| -- | -- |
| `PublishLocal(topic, from, payload)` | Delivers to local MQ subscribers in the same process. Used for browser SSE events (peer:announce, listen state, etc.). No P2P hop. |
| `Send(ctx, peerID, topic, payload)` | Sends to a remote peer over P2P stream with ACK. Returns error on timeout or delivery failure. |
| `PublishPeerAnnounce(payload)` | Convenience wrapper: `PublishLocal(TopicPeerAnnounce, "", payload)` |
| `PublishPeerGone(peerID)` | Convenience wrapper: `PublishLocal(TopicPeerGone, "", payload)` |

## Manager internals

`internal/mq/manager.go`:

- **inbox**: Per-peer in-memory buffer (cap: 200) for messages that arrive before the browser SSE connects
- **listeners**: SSE listener channels (cap: 256 per listener) — sized for ICE candidate bursts
- **pending**: ACK channels keyed by message ID
- **topicSubs**: Prefix-based topic subscribers (used by group manager, chat manager, etc.)
- **seq**: Atomic monotonic counter for outbound message ordering
