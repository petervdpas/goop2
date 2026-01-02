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
* Run in **rendezvous-only** mode (no peer node)

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
