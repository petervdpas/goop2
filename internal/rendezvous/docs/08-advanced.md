# Advanced Topics

## Running behind a reverse proxy

A rendezvous server is typically placed behind a reverse proxy like Caddy or Nginx to provide HTTPS and a custom domain. Set `external_url` so peers see the correct public address:

```json
{
  "presence": {
    "external_url": "https://goop2.com",
    "rendezvous_port": 8787
  }
}
```

The reverse proxy should forward traffic to the rendezvous port (default `8787`). Example Caddy configuration:

```
goop2.com {
    reverse_proxy localhost:8787
}
```

## Port forwarding and direct connections

By default, libp2p picks a random port for peer-to-peer connections. If you're behind a router and want reliable direct connections (avoiding relay), forward a fixed port:

```json
{
  "p2p": { "listen_port": 4001 }
}
```

Then forward port `4001` (TCP) on your router to your machine. This allows other peers to connect directly without needing the circuit relay.

## Circuit relay tuning

The relay runs alongside the rendezvous server and helps peers behind NAT reach each other. It only forwards encrypted traffic and cannot read the content.

```json
{
  "presence": {
    "relay_port": 4001,
    "relay_key_file": "data/relay.key"
  }
}
```

Peers discover the relay automatically via the rendezvous server's `/relay` endpoint. When a direct connection fails, libp2p falls back to the relay and then attempts hole-punching (DCUtR) to upgrade to a direct connection.

## Running multiple peers

You can run multiple peers on the same machine by giving each a separate directory and viewer port:

```bash
goop2 -dir peers/alice
goop2 -dir peers/bob
```

Each peer gets its own `goop.json`, identity key, database, and site directory. Set different `viewer.http_addr` ports to avoid conflicts.

## Backup and migration

All peer state lives in a single directory:

| Path | Contains |
|------|----------|
| `goop.json` | Configuration |
| `data/identity.key` | Persistent peer identity (your Peer ID) |
| `data/peers.db` | Registration and peer database (rendezvous only) |
| `site/` | Your site files and database |

To back up or migrate a peer, copy the entire directory. The `identity.key` is what determines your Peer ID -- if you lose it, you get a new identity.

## Exposing your site to the regular web

The Goop2 viewer already serves your site over plain HTTP at paths like:

```
http://127.0.0.1:8080/p/<peer-id>/
```

This works in any regular browser -- visitors don't need Goop2 installed. That means **anyone with a laptop can run a fully interactive website** and share it with the world. A chess game, a quiz for your class, a kanban board for your team, a community corkboard -- just pick a template, start your peer, and share the link.

By default the viewer binds to `127.0.0.1`, which limits access to your own machine. To open it up:

**1. Bind to all interfaces:**

```json
{
  "viewer": {
    "http_addr": "0.0.0.0:8080"
  }
}
```

**2. Optionally, put a reverse proxy in front for HTTPS and a custom domain:**

```
mysite.example.com {
    reverse_proxy localhost:8080
}
```

Visitors can then reach your site at `https://mysite.example.com/p/<peer-id>/` using any browser. Add a redirect rule in your reverse proxy to map `/` to `/p/<your-peer-id>/` for a cleaner URL.

**Why this is powerful:**

- **Zero deployment** -- no hosting provider, no containers, no CI/CD. Just run your peer.
- **Zero cost** -- your laptop is the server. As long as it's on, your site is live. Goop2 runs just fine on a Raspberry Pi too -- perfect for an always-on community site with minimal power draw.
- **Fully interactive** -- forms, real-time games, comments, leaderboards all work. Data operations are proxied through the viewer to your local database.
- **Ephemeral by nature** -- close your laptop and the site vanishes. No data left behind on someone else's server.

This makes Goop2 ideal for **temporary or community-driven sites**: a teacher running a quiz during class, a club sharing a pinboard for an event, a game night with friends, or a small team collaborating on a kanban board. No infrastructure needed -- just a peer and a link.

**Things to keep in mind:**

- Your site is only reachable while your peer is running.
- Your upload speed and hardware determine how many visitors you can handle.
- If you're behind a router, you may need to forward the viewer port or use the circuit relay for connectivity.
- For visiting **other** peers' sites through your viewer, your peer must be connected to them (via LAN or rendezvous). The viewer acts as a bridge between HTTP and the P2P network.
