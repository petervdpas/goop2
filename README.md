# Goop² — Ephemeral Web

**Goop²** is a peer-to-peer, presence-based web system for **small, personal websites**.

Each peer is a self-contained node with its own identity, content, and optional local web UI. Peers discover each other automatically, exchange presence information, and can securely serve static sites to one another **directly**, without central hosting, accounts, or permanent infrastructure.

A Goop site exists **only while its owner is present**.

This is deliberate.

---

## Vision

Goop² revives the spirit of early personal web publishing—small sites, hand-crafted pages, experimentation, individuality—while removing everything that made the modern web brittle:

* Centralized hosting
* Permanent deployment
* Global indexing
* Platform dependency

A Goop site is not *uploaded*.
It is **presented while the peer is present**.

When the peer goes offline, the site disappears.

---

## Core Concepts

### Peer

A **peer** is the fundamental unit in Goop².

* One peer = one folder + one config + one cryptographic identity
* A peer owns a strict filesystem boundary (`site/`)
* A peer may:

  * Publish presence on the network
  * Serve a static site
  * Run a local viewer and editor
  * Optionally host or connect to a rendezvous server

Running multiple peers means running multiple **independent processes**, each with its own scope and identity.

---

### Ephemeral Presence

Presence is **soft state**:

* Peers periodically announce themselves (`online`, `update`)
* Absence is inferred by timeout (`offline` or TTL expiry)
* No durable global registry exists

Presence operates on:

* **LAN** — via mDNS + libp2p pubsub
* **WAN** — optionally via rendezvous servers

“Public” never means globally indexed.
It only means *visible to peers within the chosen discovery scope*.

---

## Architecture Overview

```bash
┌─────────────┐
│  Peer A     │
│             │
│  libp2p     │◀──────────────▶ libp2p
│  Presence   │                 Presence
│  Site FS    │                 Site FS
│  Viewer UI  │                 Viewer UI
└──────┬──────┘
       │
       │ (optional WAN)
       ▼
┌──────────────────┐
│ Rendezvous Server │
│  (HTTP + SSE)     │
└──────────────────┘
```

Each peer:

* Publishes presence
* Maintains a live peer table
* Serves its site over libp2p streams
* Optionally exposes a **local-only** HTTP viewer/editor

---

## Major Components

### 1. P2P Layer (`internal/p2p`)

* Built on **libp2p**
* mDNS for LAN discovery
* GossipSub for presence
* Custom stream protocols:

  * `/goop/content/1.0.0` — lightweight peer metadata
  * `/goop/site/1.0.0` — static site file transfer

Peers fetch content **directly from each other**.
No HTTP is required between peers.

---

### 2. Presence & Rendezvous (`internal/rendezvous`)

* Optional HTTP rendezvous server
* Used to bridge LANs and enable WAN discovery
* Provides:

  * `/publish` — presence ingestion
  * `/events` — Server-Sent Events stream
  * `/peers.json` and human-readable UI

A peer can:

* Host a rendezvous server
* Join one or more WAN rendezvous meshes
* Run in **rendezvous-only** mode (no peer node, with a minimal settings viewer)

#### Standalone Rendezvous Server

The rendezvous server can be run as a standalone service for production deployments:

```bash
# Build standalone server
go build -o rendezvous ./cmd/rendezvous

# Run with options
./rendezvous -addr 127.0.0.1:8787
```

The standalone server provides:

* Real-time web monitoring UI showing connected peers
* REST API endpoints (`/peers.json`, `/healthz`)
* SSE event stream for live updates
* Designed to run behind reverse proxy (Caddy, Nginx)

See [docs/RENDEZVOUS_DEPLOYMENT.md](docs/RENDEZVOUS_DEPLOYMENT.md) for full deployment guide including:

* Systemd service configuration
* Caddy/Nginx reverse proxy setup
* Security hardening
* Load balancing for high-availability

---

### 3. Content Store (`internal/content`)

A hardened filesystem abstraction for peer sites.

Features:

* Strict root confinement (cannot escape `site/`)
* Atomic writes
* SHA-256 ETags
* Conflict detection
* Safe directory creation
* Deterministic tree and listing APIs

This store backs:

* The editor
* The local site
* The self-served peer shortcut

Raw authoring files are never exposed to other peers.

---

### 4. Viewer & UI (`internal/viewer`, `internal/ui`)

The **viewer** is a local HTTP server bound to `127.0.0.1`.

It provides:

* Peer list
* Peer content inspection
* Static site proxy (`/p/<peerID>/…`)
* Live editor
* Live logs
* Settings UI

**Minimal viewer** — Rendezvous-only peers run a lightweight variant of the viewer with only Settings and Logs routes. This allows configuring a rendezvous server through the same UI without starting a full P2P node.

Security properties:

* Viewer binds to localhost only
* Editor routes reject non-loopback requests
* Peer sites are sandboxed with strict CSP
* Browser caching is disabled (live-edit semantics)

The UI is a **control surface**, not the system itself.

---

### 5. Editor

The built-in editor allows live editing of a peer’s site:

* Tree view + directory navigation
* Create, rename, delete files and folders
* Optimistic concurrency using ETags
* No iframes, no background syncing
* Changes are immediately reflected in the served site

---

### 6. Logs

Each peer maintains an in-memory log buffer:

* Ring buffer
* JSON snapshot endpoint
* Server-Sent Events live tail
* Viewable in the UI

---

### 7. Desktop Launcher (Wails)

The desktop launcher provides:

* Peer management (create, delete, start)
* Embedded frontend assets
* Theme synchronization
* Rendezvous peer differentiation — rendezvous-only peers are marked with a badge and open directly to the Settings page instead of the peer list
* A bridge between:

  * Desktop launcher
  * Local viewer
  * Shared UI state (`data/ui.json`)

The launcher does not own system logic; it controls it.

---

## Directory Layout (Conceptual)

```bash
peers/
  peerA/
    goop.json
    site/
      index.html
      assets/
  peerB/
    goop.json
    site/
data/
  identity.key
  ui.json
```

Each peer directory is **fully isolated**.

---

## Configuration (`goop.json`)

Key sections:

* **identity**

  * Key storage
* **paths**

  * Site root, source, staging
* **p2p**

  * Listen port
  * mDNS tag
* **presence**

  * TTL / heartbeat
  * Rendezvous options
  * `rendezvous_only` — run as a dedicated rendezvous server without a P2P node
* **viewer**

  * Local HTTP bind address

Configuration validation is strict and defensive.

---

## Running a Peer

Typical lifecycle:

1. Create a peer folder
2. Ensure or edit `goop.json`
3. Start the peer
4. The peer:

   * Joins the presence mesh
   * Serves its site
   * Starts the viewer (if enabled)

Multiple peers can run simultaneously on one machine.

---

## Deployment Options

### Desktop Application (Wails)

Build and run the full desktop application with GUI:

```bash
# Development
wails dev

# Production build
wails build

# Run the built app (from build/bin/)
./build/bin/goop2
```

The desktop app provides a visual interface for managing multiple peers, creating new peers, and controlling them from a unified UI.

**Note:** The desktop UI requires building with `wails build` or `wails dev`. A binary built with plain `go build` can only run CLI commands (peer/rendezvous modes).

---

### CLI Peer Mode

Run a peer from the command line:

```bash
# Build CLI-capable binary
go build -o goop2

# Run a peer in CLI mode
./goop2 peer ./peers/mysite

# Run a peer configured as rendezvous server
./goop2 rendezvous ./peers/server
```

**Use cases:**
* Server deployments (systemd services)
* Headless environments
* Automation and scripting
* Docker containers
* Lower resource usage (no GUI overhead)

**What it does:**
* Loads configuration from `<dir>/goop.json`
* Serves static site from `<dir>/site/`
* Joins P2P network and announces presence
* Starts local viewer (if configured)
* Optionally hosts rendezvous server (if configured)

See [docs/CLI_TOOLS.md](docs/CLI_TOOLS.md) for systemd service examples, production deployment patterns, and container configurations.

---

### Rendezvous Server Mode

Run as a dedicated rendezvous server using the same executable:

```bash
# Build (same executable)
go build -o goop2

# Run in rendezvous mode
./goop2 -rendezvous -addr 127.0.0.1:8787
```

When a peer is configured with `rendezvous_only: true`, it runs only the rendezvous server (no P2P node). A **minimal settings viewer** is started alongside the rendezvous server, providing:

* Settings page for configuring the peer label, email, and rendezvous options
* Logs page for monitoring rendezvous activity
* Link to the rendezvous dashboard

In the desktop launcher, rendezvous-only peers display a "Rendezvous" badge. Starting one opens the Settings page directly.

**Production Deployment:**

* Systemd service for automatic restart and daemon management
* Reverse proxy (Caddy/Nginx) for HTTPS and security
* Web UI for real-time peer monitoring
* Load balancing support for high availability

See [docs/RENDEZVOUS_DEPLOYMENT.md](docs/RENDEZVOUS_DEPLOYMENT.md) for complete deployment guide.

**Example Caddy Configuration:**

```caddyfile
rendezvous.yourdomain.com {
    reverse_proxy localhost:8787
}
```

See [docs/Caddyfile.example](docs/Caddyfile.example) for more configurations.

---

## Design Principles

* **Ephemeral by default**
* **Peer-first**
* **No global state**
* **Filesystem boundaries are sacred**
* **Local-only administrative surfaces**
* **Explicit networking**
* **Composable, not monolithic**
* **Human-scale by design**

---

## What Goop² Is *Not*

* Not a social network
* Not a blogging platform
* Not a hosting provider
* Not a permanent web
* Not optimized for SEO, scale, or monetization

Goop² is closer to a **temporary, living web of peers**.

---

## Status

Goop² is a working system with:

* Real networking
* Real UI
* Real filesystem safety
* Real peer interaction

It is intentionally opinionated and minimal.

---

## License

Goop² is licensed under the **GNU General Public License v2.0 only**,
the same license used by the Linux kernel.

See `COPYING` for the full license text.

---

**Goop² — the web, while it exists.**
