# Connecting to Peers

Goop2 supports two discovery mechanisms that can be used independently or together.

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

## Email registration

A rendezvous server can require email verification before peers are allowed to be discovered. When enabled, new peers must register and verify their email address before their presence is visible to others:

```json
{
  "presence": {
    "registration_required": true,
    "peer_db_path": "data/peers.db"
  }
}
```

Visitors can register at the `/register` page on the rendezvous server. A verification link is sent via email (if SMTP is configured) or logged to the console for testing.

## Visiting peers

Once peers are connected (via LAN or rendezvous), they appear in your viewer. Click on a peer to visit their site. The URL pattern is:

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
