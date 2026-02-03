# Super Hubs

A super hub is a directory of communities. Each community is served by a rendezvous server. A super hub knows which communities exist, where they live, and whether they're healthy. It does not handle peer traffic. It does not store messages. It is a phonebook, not a telephone exchange.

---

## Three Layers

A super hub does three things. They are independent concerns.

### 1. Directory

The super hub maintains a list of communities:

```bash
Community: "Go Developers"
  rendezvous: rv1.example.com:8787
  type: open
  members: 142

Community: "Chess Club"
  rendezvous: rv2.example.com:8787
  type: invite-only
  members: 23
```

Peers query the directory to discover communities they can join. This is the public-facing role.

### 2. Administration

The super hub operator runs rendezvous servers. The super hub configures, starts, stops, and updates them. This is a private tree — one operator, many servers:

```bash
super hub (operator: Alice)
  ├── rv1.example.com  (3 communities)
  ├── rv2.example.com  (1 community)
  └── rv3.example.com  (standby)
```

Alice controls her servers. She sets admin passwords, allocates ports, assigns communities to servers. No one else touches her infrastructure.

### 3. Monitoring

The super hub queries its rendezvous servers for health and stats. Each rendezvous server already has an `/admin` API protected by a password. The super hub knows the passwords for its own servers, so it can:

- Check if each server is alive
- Count connected peers per community
- Detect overloaded or failed servers
- Present a unified dashboard

This is the admin plane reaching downward.

---

## Federation

Federation is what makes multiple super hubs appear as one directory to peers. This is the only hard problem.

### What federation is not

Federation does not merge super hubs into a single system. There is no master. There is no global state. Each super hub remains independent and authoritative over its own communities.

### What federation is

Federation is gossip. Super hubs that choose to federate share their directory entries with each other. A peer can ask any federated super hub and get results from all of them.

### How it works

Each directory entry has an origin:

```json
{
  "community": "Go Developers",
  "rendezvous": "rv1.example.com:8787",
  "origin": "superhub.alice.com",
  "updated": "2026-02-03T12:00:00Z"
}
```

When super hubs sync:

1. Alice's super hub sends its entries to Bob's super hub
2. Bob's super hub sends its entries to Alice's
3. Both now have the full combined directory
4. Each entry retains its origin — Alice's communities are still hers, Bob's are still his
5. Only the origin super hub can update or delete an entry

This is eventual consistency. When Alice adds a new community, Bob sees it after the next sync. When Alice removes one, Bob removes it too. No coordination required. No leader election. No consensus protocol.

### Trust

Federation requires trust decisions:

- **Who do you federate with?** Each super hub operator explicitly chooses peers. No automatic discovery.
- **Do you show their communities?** A super hub can federate for replication (redundancy) without displaying the other's communities in its own directory.
- **What if someone lists spam communities?** You stop federating with them. Their entries expire and disappear from your directory.

### The DNS parallel

```bash
DNS                         Goop
─────────────────────────────────────────
Root servers          ←→    Super hubs (federated)
TLD registrars        ←→    Super hub operators
Domain names          ←→    Community names
A/AAAA records        ←→    Rendezvous server addresses
Zone transfers        ←→    Federation sync
```

DNS doesn't have "one directory." It has thousands of nameservers that collectively answer any query by delegation. Super hubs work similarly, but simpler — flat gossip instead of hierarchical delegation.

---

## Perspectives

### The peer (user)

A peer sees a single search box. They type "chess" and get results. They don't know or care which super hub answered. They don't know if the result was local or federated. They pick a community, their client connects to the rendezvous server listed, and they're in.

```bash
Peer types "chess"
  → asks their configured super hub
  → super hub returns results (own + federated)
  → peer picks "Chess Club"
  → peer connects to rv2.example.com:8787
  → peer is now in the community
```

The super hub is never in the data path after discovery.

### The community operator

A community operator wants to run a community. They either:

1. **Self-host**: Run their own rendezvous server and register with a super hub
2. **Managed**: Pay a super hub operator to host it for them

Either way, they get a listing in the directory. If the super hub federates, their community is discoverable from other super hubs too.

### The super hub operator

The super hub operator runs infrastructure:

- One super hub service (the directory + admin dashboard)
- N rendezvous servers (the community endpoints)
- Federation links to other super hub operators (optional)

Their dashboard shows:

```bash
My Communities (3 rendezvous servers, 5 communities)
  rv1: healthy, 142 peers across 3 communities
  rv2: healthy, 23 peers across 1 community
  rv3: standby, 0 peers

Federated (2 peers)
  superhub.bob.com: 12 communities, last sync 30s ago
  superhub.carol.com: 8 communities, last sync 2m ago

Total directory: 25 communities
```

### The developer

Three protocols to implement:

1. **Directory API** — CRUD for community listings, search/browse for peers
2. **Admin aggregation** — Poll rendezvous `/admin` endpoints, aggregate stats
3. **Federation sync** — Bilateral entry exchange between super hubs, origin-based conflict resolution, expiry of stale entries

The directory API and admin aggregation are straightforward REST. Federation sync can start as a simple pull-based approach (each super hub periodically fetches the other's directory) and evolve to push-based (SSE or WebSocket) later.

---

## What a super hub is not

- **Not a relay.** It never touches peer-to-peer traffic.
- **Not a host.** It does not store community data.
- **Not required.** Peers can connect to rendezvous servers directly without any super hub.
- **Not a single point of failure.** If a super hub goes down, communities continue to function. Peers already connected stay connected. Only new discovery is affected, and federated super hubs still serve the directory.
- **Not centralized.** Multiple super hubs with federation means no single operator controls the directory.
