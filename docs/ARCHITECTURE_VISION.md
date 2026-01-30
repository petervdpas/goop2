# The Goop² Manifesto

## The Web Was Supposed to Be Ours

The original web was a network of equals. Every participant could be both reader and publisher, client and server. There were no gatekeepers, no platforms, no terms of service. Your data lived on your machine, and the network existed because people chose to be part of it.

That web is gone. Today, your data lives on someone else's server. Your identity is owned by a corporation. Your content persists because a company pays the hosting bill — and disappears when they decide it should. The web became a handful of platforms masquerading as the open internet.

Goop² is a return to the original vision — rebuilt with modern tools.

---

## What Goop² Is

Goop² is a peer-to-peer desktop application that creates a decentralized web — a network where every participant is both client and server. Each peer runs a local application with its own database, its own web UI, and its own cryptographic identity.

We call it the **ephemeral web**: a web that exists only as long as its peers are alive, owned by no one centrally.

Every peer is a platform unto itself:

- **A web server** — serving its own ephemeral UI, accessible by other peers
- **A database** — local SQLite, queryable, editable, owned entirely by the user
- **An identity** — cryptographic key pair + verified email, self-sovereign
- **A node in the mesh** — discovering, communicating, and collaborating with other peers

This is not an app. It is infrastructure for a new kind of web.

---

## Communities, Not Platforms

In today's web, communities exist at the mercy of platforms. A Discord server, a Subreddit, a Facebook Group — none of them belong to the people in them. The platform owns the space, the data, and the rules. It can shut down, change terms, or sell your attention to advertisers at any time.

In Goop², **a community is a rendezvous server**. That is the fundamental equation.

A community hub is a lightweight server that handles peer discovery — helping peers find each other without sharing addresses manually. It provides NAT traversal, relay services, and a persistent meeting point. But it does not own the data. It does not host the conversations. It does not control the peers.

The hub is the town square. The peers are the people. When the square closes, the people still exist — and they can meet somewhere else.

### Community Hubs

Each community hub is independent and sovereign:

- **Membership**: open, invite-only, or application-based
- **Economics**: free, paid, or donation-based
- **Governance**: the hub operator sets and enforces the rules
- **Scope**: peer discovery and broadcast chat are per-community
- **Identity**: peers verify their email to join — real identity, not throwaway accounts

A peer can belong to multiple communities simultaneously. Different communities for different contexts — just like real life.

### Super Hubs: The Directory Layer

Above community hubs sit **super hubs** — directories that answer the question: *what communities exist?*

A super hub maintains a registry of community hubs: their names, descriptions, addresses, and membership types. It lets peers browse and search for communities to join. It does not relay traffic or know about individual peers. It is lightweight, cheap to run, and can federate with other super hubs.

Super hubs are **DNS for communities**:

```
Super Hub (directory)
  ├── Community Hub: "Go Developers"
  │     ├── peer-A
  │     ├── peer-B
  │     └── peer-C
  ├── Community Hub: "Music Production"
  │     ├── peer-D
  │     └── peer-A  (same peer, multiple communities)
  └── Community Hub: "Private Team X"  (invite-only, unlisted)
        ├── peer-E
        └── peer-F
```

Multiple super hubs can exist and sync their directories with each other. No single point of control. No single point of failure. Decentralized at every level.

---

## The Economics of Freedom

Goop² is free software built on a simple economic principle: **you only pay when you use infrastructure that costs money to run.**

### Free: Direct Connections

Two peers can always connect directly if they know each other's addresses. No infrastructure required, no cost to anyone. Full functionality — chat, database, ephemeral web, everything. This is raw libp2p, peer-to-peer in its purest form.

Direct connections keep the project honest. Users are never *forced* through paid infrastructure. The paid tiers provide genuine value, not artificial gatekeeping.

### $9.99/Year: Community Hub Access

For the price of a coffee, a peer gets:

- Automatic discovery within a community (no manual address sharing)
- NAT traversal and relay services
- Persistent availability across sessions
- Verified email identity
- Access to community broadcast chat and peer directory

This is the tier where most users will live. It funds the infrastructure that makes communities work.

### Premium: Hub Hosting

For organizations and community builders:

- Managed community hub infrastructure ("we run the server")
- Custom configuration, moderation tools
- Uptime guarantees

### Super Hub Listings

Community hub operators can pay to be listed and featured in the super hub directory — promoted placement, verified badges, visibility.

---

## A Web Inside the Web

The parallels to the traditional web are not accidental:

| Traditional Web | Goop² Ephemeral Web |
|---|---|
| DNS root servers | Super hubs |
| Domain registrars | Community hubs |
| Web servers | Peers |
| Browsers | Goop² viewer |
| Databases | SQLite per peer |
| HTTP/HTTPS | libp2p protocols |
| Domain names | Verified email + peer ID |
| Web hosting | Hub hosting (premium tier) |
| Search engines | Super hub directories |
| Social media platforms | Community hubs with broadcast |

But there is a fundamental difference.

In the traditional web, content lives on someone else's server and persists because a corporation pays the hosting bill. Your data is the product. Your identity is rented.

In the ephemeral web, data lives with the user. Identity is self-sovereign. The network exists because people choose to participate — and it vanishes when they leave. Nothing persists without consent. Nothing is owned without agency.

**The web was supposed to be ours. Goop² makes it ours again.**

---

## Identity and Trust

In a decentralized network, identity is the hardest problem. Goop² takes a pragmatic approach:

- **Cryptographic identity**: Every peer has a libp2p key pair. This is the foundation — unforgeable, verifiable, permanent.
- **Email verification**: Required for hub access. Ties the cryptographic identity to something human-meaningful. Verified through a confirmation code flow during registration.
- **Community reputation**: Trust is built through behavior within a community. Peers that have been present longer, communicated reliably, and contributed meaningfully earn trust organically.
- **Blocklists and allowlists**: Every peer can curate who they interact with. Simple, effective, user-controlled.

No central identity provider. No "login with Google." Your identity is yours.

---

## What Can Be Built

Because every peer is a web server with a database, Goop² is a foundation for applications that don't exist yet:

- **Decentralized marketplaces** — peers list goods/services, transact directly
- **Collaborative wikis** — community knowledge bases, replicated across peers
- **Distributed social networks** — posts, follows, feeds — without a platform
- **Private team workspaces** — invite-only communities with shared data
- **IoT mesh networks** — devices as peers, communicating without cloud services
- **Offline-first applications** — everything works locally, syncs when connected

The SQLite-per-peer model means every user has a programmable data store. The ephemeral web UI means every user has a customizable interface. The libp2p mesh means every user is connected to every other user who chooses to be found.

This is not a chat app with a database viewer. This is infrastructure for a new kind of internet.

---

## The Road Ahead

1. **Email verification** — Required identity for hub access
2. **Multi-hub support** — Peers join multiple communities simultaneously
3. **Community-scoped UI** — Peer lists and broadcast filtered by active community
4. **Subscription gating** — Hubs verify email and subscription before allowing discovery
5. **Super hub protocol** — Directory API for registering, searching, and listing hubs
6. **Browse Communities view** — UI for discovering and joining communities
7. **Hub federation** — Super hubs sync directories with each other
8. **Application layer** — SDKs and templates for building on top of the peer platform

---

*Goop² — the ephemeral web.*
