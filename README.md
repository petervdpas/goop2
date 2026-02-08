<p align="center">
  <img src="frontend/src/assets/images/goop2-splash.png" alt="Goop2" width="360" />
</p>

<h1 align="center">Goop²</h1>
<p align="center"><strong>The web, while it exists.</strong></p>
<p align="center">
  A peer-to-peer platform for ephemeral websites, apps, and services.<br/>
  Your content lives while you're online. When you leave, it disappears.
</p>

<p align="center">
  <a href="https://goop2.com/docs">Docs</a> &middot;
  <a href="https://goop2.com">Live Rendezvous</a> &middot;
  <a href="#quick-start">Quick Start</a>
</p>

---

## What is Goop²?

Goop² lets you run websites, apps, and services that exist only while you're present. No hosting, no deployment, no accounts. Just you and your peers.

- **Peer-to-peer** — sites are served directly between computers using [libp2p](https://libp2p.io/), no server in between
- **Ephemeral** — your site appears when you come online and vanishes when you leave
- **LAN + WAN** — peers find each other automatically on local networks (mDNS) or across the internet via rendezvous servers
- **Built-in editor** — create and edit your site live from a local web UI
- **Templates** — install ready-made sites (chess, kanban, quiz, photo gallery...) with one click
- **Data-capable** — templates can include databases, forms, and real-time interaction between peers
- **NAT traversal** — circuit relay + hole punching so peers behind routers can still connect
- **Desktop app** — manage multiple peers from a native launcher (built with [Wails](https://wails.io/))

## How it works

```
You (Peer A)                    Friend (Peer B)
┌──────────┐                    ┌──────────┐
│ site/    │◄──── libp2p ─────►│ site/    │
│ data.db  │    direct P2P      │ data.db  │
│ editor   │                    │ editor   │
└────┬─────┘                    └────┬─────┘
     │                               │
     └──── rendezvous server ────────┘
           (optional, for WAN discovery)
```

Each peer is a self-contained node: a folder with your site files, a config, and a cryptographic identity. Peers discover each other, exchange presence, and serve content directly. The rendezvous server is optional — it just helps peers find each other across the internet.

## Quick start

### Desktop app (recommended)

```bash
# requires Go 1.24+ and Wails CLI
wails build
./build/bin/goop2
```

The launcher lets you create peers, start/stop them, and open their editors.

### CLI

```bash
go build -o goop2

# run a peer
./goop2 peer ./peers/mysite

# run a rendezvous server
./goop2 rendezvous ./peers/server
```

### Rendezvous server (production)

```bash
go build -o rendezvous ./cmd/rendezvous
./rendezvous -addr 127.0.0.1:8787
```

Put it behind Caddy or Nginx for HTTPS. See [docs/goop-rendezvous.service](docs/goop-rendezvous.service) for systemd and [docs/Caddyfile.example](docs/Caddyfile.example) for reverse proxy config.

## Templates

Peers connected to a rendezvous server can browse and install site templates:

| Template | Description |
|---|---|
| Chess | Play chess with connected peers |
| Kanban | Collaborative task board |
| Corkboard | Community bulletin board |
| Quiz | Trivia and quizzes |
| Photobook | Photo gallery |
| Arcade | Browser games |

Templates include HTML, CSS, JS, database schemas, and optional Lua server functions. You can also build your own.

## Architecture

Goop² is built in Go with a four-service microservice architecture:

| Service | Port | Purpose |
|---|---|---|
| **goop2** (this repo) | 8787 | Gateway — rendezvous, proxying, HTML serving |
| **goop2-registrations** | 8801 | Email verification, registration DB |
| **goop2-credits** | 8800 | Credit balances, pricing, template ownership |
| **goop2-email** | 8802 | SMTP sending, HTML email templates |

Main packages in this repo:

| Package | What it does |
|---|---|
| `internal/p2p` | libp2p networking, stream protocols, presence |
| `internal/rendezvous` | HTTP rendezvous server (SSE, template store, relay) |
| `internal/content` | Sandboxed filesystem for peer sites |
| `internal/storage` | SQLite database per peer |
| `internal/viewer` | Local HTTP UI (editor, peer browser, settings) |
| `internal/lua` | Lua scripting engine for data functions |
| `internal/realtime` | WebRTC video/audio between peers |
| `internal/config` | Configuration and validation |

For the full deep-dive, see the [Architecture docs](https://github.com/petervdpas/goop2-email/tree/master/docs).

## Project structure

```
peers/           # peer data directories (one folder per peer)
  mysite/
    goop.json    # config
    data.db      # SQLite database
    site/        # your website files
cmd/             # standalone binaries (rendezvous server)
internal/        # core Go packages
frontend/        # desktop launcher UI (Wails + JS)
docs/            # deployment guides, architecture docs
```

## Contributing

Goop² is open source under **GPLv2**. Contributions are welcome.

1. Fork the repo
2. Create a feature branch
3. Submit a pull request

## License

GNU General Public License v2.0 — the same license used by the Linux kernel.
See [COPYING](COPYING) for the full text.

---

<p align="center"><strong>Goop²</strong> — the web, while it exists.</p>
