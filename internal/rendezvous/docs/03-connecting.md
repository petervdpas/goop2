# Connecting to Peers

Goop2 supports three connection methods that can be used independently or together.

## LAN discovery (mDNS)

On a local network, peers find each other automatically via mDNS. No configuration is needed -- just start two or more peers on the same LAN and they will appear in each other's viewer within seconds.

Under the hood, Goop2 uses libp2p GossipSub to broadcast presence messages. The `mdns_tag` in your config determines which group of peers can see each other:

```json
{
  "p2p": {
    "mdns_tag": "goop-mdns"
  }
}
```

All peers sharing the same `mdns_tag` will discover each other on the local network. If you want to run separate clusters on the same LAN, use different tags.

### Requirements

- All peers must be on the same broadcast domain (same subnet).
- UDP port 5353 must not be blocked by your firewall.
- Virtual machines and Docker containers may need bridged networking.

## WAN discovery (rendezvous server)

To connect peers across different networks, use a rendezvous server. The rendezvous server is a lightweight HTTP service that peers publish their presence to and subscribe for updates via Server-Sent Events.

### Option A: Connect to an existing server

If someone is already running a rendezvous server, add its URL to your peer's config:

```json
{
  "presence": {
    "rendezvous_wan": "https://goop2.com"
  }
}
```

Restart your peer and it will begin publishing its presence to that server.

### Option B: Host your own rendezvous server

Any Goop2 peer can act as a rendezvous server. Enable it in your config:

```json
{
  "presence": {
    "rendezvous_host": true,
    "rendezvous_port": 8787,
    "rendezvous_bind": "0.0.0.0",
    "admin_password": "your-secret-password"
  }
}
```

The server will be available at `http://<your-ip>:8787`. The public page shows connect URLs that other peers can paste into their `rendezvous_wan` setting.

### Rendezvous-only mode

If you want to run a dedicated rendezvous server without a P2P node (for example, on a VPS), use rendezvous-only mode:

```json
{
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787,
    "rendezvous_bind": "0.0.0.0",
    "admin_password": "your-secret-password"
  }
}
```

Start it with:

```bash
./goop2 rendezvous ./peers/server
```

### Production deployment

For a production rendezvous server accessible over the internet, put it behind a reverse proxy with TLS and set the `external_url` so peers see the public address:

```json
{
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787,
    "rendezvous_bind": "0.0.0.0",
    "admin_password": "your-secret-password",
    "external_url": "https://goop2.com"
  }
}
```

```
# Example Caddyfile
goop2.com {
    reverse_proxy localhost:8787
}
```

## Bridge mode (thin client)

For environments where running a full libp2p node is not practical, Goop2 supports a **bridge mode**. A thin-client peer connects through a bridge service over WebSocket instead of establishing direct P2P connections.

```json
{
  "p2p": {
    "bridge_mode": true
  },
  "presence": {
    "bridge_url": "http://localhost:8804"
  },
  "profile": {
    "email": "me@example.com"
  }
}
```

The bridge service must be running alongside the rendezvous server (see the [goop2-services](https://github.com/petervdpas/goop2-services) repository). The thin client authenticates with a bridge token and appears as a virtual peer to other peers on the network.

Bridge mode is useful for:

- Web-only clients that cannot run libp2p.
- Restricted network environments that block P2P traffic.
- Lightweight devices where a full P2P stack is too heavy.

## NAT traversal

When peers are on different networks behind NAT routers, direct connections aren't always possible. Goop2 handles this automatically using two mechanisms:

### Hole punching (DCUtR)

Goop2 uses libp2p's Direct Connection Upgrade through Relay (DCUtR) protocol. When two peers can't connect directly, they coordinate through a relay to punch a hole through their NATs. If hole punching succeeds, subsequent traffic flows directly between peers.

### Circuit relay

If hole punching fails, traffic flows through a **circuit relay** -- a lightweight proxy that forwards data between peers. A rendezvous server can run a circuit relay alongside its discovery service:

```json
{
  "presence": {
    "rendezvous_host": true,
    "relay_port": 4001
  }
}
```

Peers automatically discover the relay via the rendezvous server's `/relay` endpoint and use it when needed. The relay only forwards encrypted traffic; it cannot read the content.

Relay timing can be tuned for your network conditions:

```json
{
  "presence": {
    "relay_cleanup_delay_sec": 3,
    "relay_poll_deadline_sec": 10,
    "relay_connect_timeout_sec": 5,
    "relay_refresh_interval_sec": 90,
    "relay_recovery_grace_sec": 5
  }
}
```

## Encryption

When an encryption service is configured, peers exchange NaCl public keys through the rendezvous server. This enables end-to-end encryption for P2P messages and broadcast key distribution for group communications.

Encryption keys are generated automatically on first use and stored in the peer's config (`nacl_public_key` / `nacl_private_key`).

## Email registration

A rendezvous server can require email verification before peers are allowed to be discovered. This is managed by the **registration service** -- a standalone microservice that handles email verification and the registration database.

To enable registration, set `use_services` to `true` and point your goop2 config at the registration service:

```json
{
  "presence": {
    "use_services": true,
    "registration_url": "http://localhost:8801",
    "registration_admin_token": "your-shared-token",
    "peer_db_path": "data/peers.db"
  }
}
```

The `registration_required` toggle is configured in the **registration service's** own `config.json`, not in the goop2 config. Verification emails are sent via the **email service** (configured separately in the registration service).

Visitors can register at the `/register` page on the rendezvous server.

## Visiting peers

Once peers are connected (via LAN, rendezvous, or bridge), they appear in your viewer. Click on a peer to visit their site. The URL pattern is:

```
http://127.0.0.1:8080/p/<peer-id>/
```

The viewer fetches the remote peer's site files over a direct P2P stream and renders them locally. Any data operations (form submissions, queries) are proxied to the remote peer's database.

## Discovery modes summary

| Mode | Scope | Config needed |
|------|-------|---------------|
| LAN only | Same network | None (default) |
| LAN + WAN | Multiple networks | Set `rendezvous_wan` |
| WAN only | Internet-wide | Set `rendezvous_only` + `rendezvous_host` |
| Bridge | Via bridge service | Set `bridge_mode` + `bridge_url` |
