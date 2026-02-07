# Networking

## How peers find each other

### LAN (local network)

On a local network, peers discover each other automatically via mDNS — each
peer broadcasts "I'm here" and everyone on the same WiFi/LAN sees it. No
configuration needed.

### WAN (internet)

Across the internet, peers use a **rendezvous server** — a lightweight HTTP
server that acts as a bulletin board. Every few seconds each peer posts a
presence message ("I'm online, here's my name"), and all other peers watch
for updates via a Server-Sent Events stream.

The rendezvous server does NOT carry actual data. It only handles "who's
online" messages. Peers still talk directly to each other for files, chat,
etc. via libp2p.

## Rendezvous servers

A rendezvous server exposes:

| Endpoint | Method | Purpose |
|---|---|---|
| `/publish` | POST | Peers post presence messages |
| `/events` | GET | SSE stream of presence updates |
| `/` | GET | HTML dashboard |
| `/admin` | GET | Admin dashboard (password-protected) |
| `/healthz` | GET | Health check |
| `/store` | GET | Template store web page |
| `/register` | GET/POST | Peer email registration |
| `/verify` | GET | Email verification link handler |
| `/relay` | GET | Circuit relay info (JSON) |
| `/docs/` | GET | Documentation pages |
| `/api/templates` | GET | Template store API |

The rendezvous binds to `127.0.0.1:8787` (localhost only). To make it
reachable from the internet, put it behind a reverse proxy with TLS and set
`external_url` to the public address (e.g. `https://goop2.com`).

## Multiple rendezvous servers

A peer can connect to two rendezvous servers simultaneously:

1. **Local** — if `rendezvous_host: true`, it runs its own and connects to it
2. **WAN** — if `rendezvous_wan` is set to a URL (e.g. `https://rv.example.org`)

It publishes presence to both and merges the peer lists, so you see local
peers and internet peers together.

## Rendezvous-only mode

Setting `rendezvous_only: true` turns a machine into a pure presence broker —
no site hosting, no P2P, just the bulletin board. This is what you'd run on a
dedicated server or VPS.

## Firewall / port reference

| What | Port | When to open |
|---|---|---|
| Rendezvous server | `rendezvous_port` (default 8787) TCP | If you want peers outside your LAN to use your rendezvous |
| Circuit relay | `relay_port` (e.g. 4001) TCP | If your rendezvous server runs a relay for NAT traversal |
| libp2p listener | `listen_port` (default random) TCP | For direct peer-to-peer connections from WAN |
| mDNS | 5353 UDP | LAN only, usually already allowed |

## NAT traversal

When both peers are behind NAT (home routers), direct connections aren't
always possible. Goop2 handles this automatically:

1. **Hole punching (DCUtR)** — When two peers can't connect directly, they
   coordinate through a relay to punch a hole through their NATs using
   libp2p's Direct Connection Upgrade through Relay protocol. If hole
   punching succeeds, subsequent traffic flows directly between peers.

2. **Circuit relay** — If hole punching fails, traffic flows through a
   circuit relay — a lightweight proxy that forwards encrypted data between
   peers. A rendezvous server can run a relay alongside its discovery service
   by setting `relay_port` (e.g. `4001`). Peers automatically discover the
   relay via the rendezvous server's `/relay` endpoint and use it when needed.

The relay only forwards encrypted traffic; it cannot read the content.

## Presence protocol

Each presence message is a small JSON object:

```json
{
  "type": "online|update|offline",
  "peerID": "12D3Koo...",
  "content": "Alice's node",
  "email": "alice@example.com",
  "avatarHash": "abc123",
  "ts": 1738764000000
}
```

- Peers publish a heartbeat every `heartbeat_seconds` (default 5)
- Peers are considered offline after `ttl_seconds` without a heartbeat (default 20)
- The rendezvous server prunes stale peers every 5 seconds

## Running a rendezvous on a VPS / Heroku

A rendezvous server is just HTTP + SSE — nothing exotic. It works on any
platform that can serve HTTP.

**Heroku example:**

Set `rendezvous_only: true`, bind to `0.0.0.0`, and use the port Heroku
provides via `$PORT`. The server is lightweight (tiny JSON messages every few
seconds per peer), so a basic tier is sufficient.

Then point all your peers' `rendezvous_wan` at the Heroku URL and everyone
sees everyone.

**Any VPS:**

Run goop2 in rendezvous-only mode behind a reverse proxy (see
`docs/Caddyfile.example` for a Caddy config). A systemd unit is provided in
`docs/goop-rendezvous.service`.

## Federation (planned)

The `docs/SUPERHUB.md` spec describes a future system where multiple "super
hubs" sync community directories with each other via bilateral gossip:

- Each super hub maintains a registry of communities
- Entries track their origin hub (only the origin can update/delete)
- Super hubs exchange registries peer-to-peer with explicit trust
- Eventual consistency, no leader election

This is not yet implemented. Today, the extent of "federation" is: one peer
can join one WAN rendezvous server.

## Config reference

| Field | Type | Default | Purpose |
|---|---|---|---|
| `presence.topic` | string | `"goop.presence.v1"` | GossipSub topic for LAN presence |
| `presence.ttl_seconds` | int | 20 | Seconds before a peer is considered offline |
| `presence.heartbeat_seconds` | int | 5 | Seconds between heartbeats |
| `presence.rendezvous_host` | bool | false | Run a rendezvous server locally |
| `presence.rendezvous_port` | int | 8787 | Local rendezvous server port |
| `presence.rendezvous_wan` | string | `""` | WAN rendezvous URL to join |
| `presence.rendezvous_only` | bool | false | Run only the rendezvous server, no P2P |
| `presence.admin_password` | string | `""` | Password for `/admin` endpoint |
| `presence.external_url` | string | `""` | Public URL for the server (e.g. `https://goop2.com`). Required behind a reverse proxy. |
| `presence.peer_db_path` | string | `""` | SQLite path for peer state persistence (empty = in-memory) |
| `presence.templates_dir` | string | `"templates"` | Directory for store templates (relative to peer dir) |
| `presence.registration_required` | bool | false | Require email verification before peers are discoverable |
| `presence.registration_webhook` | string | `""` | URL called (POST) when a registration is verified |
| `presence.relay_port` | int | 0 | Circuit relay port. When > 0, a relay runs alongside the rendezvous server. |
| `presence.relay_key_file` | string | `"data/relay.key"` | Path to the relay identity key file |
| `presence.smtp_host` | string | `""` | SMTP server host for verification emails |
| `presence.smtp_port` | int | 587 | SMTP server port |
| `presence.smtp_username` | string | `""` | SMTP username |
| `presence.smtp_password` | string | `""` | SMTP password or token |
| `presence.smtp_from` | string | `""` | From address for emails (defaults to `smtp_username`) |
| `p2p.listen_port` | int | 0 | libp2p TCP port (0 = random) |
| `p2p.mdns_tag` | string | `"goop-mdns"` | mDNS discovery tag |
