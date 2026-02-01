# Distributed Compute: The Ephemeral Fabric

## The Insight

Every Goop2 peer is already a compute node. It has a CPU, a database, a scripting engine, and a network identity. When a visitor calls a Lua function on a peer, the peer does work and returns a result. That's a remote procedure call. The infrastructure for distributed computation already exists — it just doesn't know it yet.

The missing piece is coordination. Today, one peer calls one function on one other peer. There's no way to say "split this work across every peer in my group" or "queue a task for a peer that's currently offline." Add those two capabilities and Goop2 becomes something unexpected: a serverless compute fabric that runs on consumer hardware.

---

## The Cloud Parallel

The architecture maps surprisingly well to managed cloud services:

| Cloud Concept | Goop2 Equivalent |
|---|---|
| Azure Service Fabric / AWS Lambda | Lua data functions on peers |
| Service registry / discovery | Peer groups + `lua-list` |
| Message queue (SQS, Service Bus) | Work queue in host's database |
| Stateful services | SQLite per peer |
| API Gateway | Data protocol proxy |
| IAM / authentication | libp2p peer identity (cryptographic) |
| Container orchestration | Peer groups with role assignment |
| Health checks / heartbeat | GossipSub presence |
| Cluster management console | Viewer UI |

The critical difference: cloud compute runs on always-on VMs in data centers that consume power 24/7. Goop2 compute runs on devices that are already turned on for other reasons — laptops, desktops, home servers. The marginal energy cost of running a Lua function on an idle laptop is effectively zero.

---

## What Exists Today

These primitives are already implemented and working:

**Lua data functions** — A peer can define `call(request)` functions in `site/lua/functions/`. These execute in a sandboxed VM with access to the peer's database via `goop.db`. They accept structured parameters and return structured results. Hot-reloaded on file change.

**Cross-peer function calls** — Any peer can invoke a data function on any other peer via the data protocol (`lua-call` operation). The caller's identity is cryptographically verified. Rate-limited and sandboxed.

**Function discovery** — The `lua-list` operation returns all available data functions on a peer, with descriptions. A coordinator can query what capabilities each peer offers.

**Peer groups** — Peers can be organized into groups with a host acting as relay. The host knows which members are online. Members can communicate in real-time via `/goop/group/1.0.0`.

**Database per peer** — Every peer has its own SQLite database. Lua functions can read and write to it. The data protocol allows remote reads and writes with identity stamping.

**Presence** — GossipSub presence tells every peer who's online. The peer table tracks connected peers with metadata.

---

## What's Missing

Three new primitives turn these building blocks into a compute fabric:

### 1. Work Queue

A table in the coordinator's database that holds work items. Each item has a target peer (or "any available"), a function to call, parameters, and a status.

```sql
CREATE TABLE _work_queue (
    _id          INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id     TEXT NOT NULL,
    function     TEXT NOT NULL,           -- Lua function to call
    params       TEXT NOT NULL DEFAULT '{}', -- JSON parameters
    status       TEXT NOT NULL DEFAULT 'pending',
                 -- pending | claimed | running | completed | failed
    assigned_to  TEXT,                    -- peer ID of the worker
    result       TEXT,                    -- JSON result
    error        TEXT,                    -- error message if failed
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    claimed_at   DATETIME,
    completed_at DATETIME,
    ttl_seconds  INTEGER DEFAULT 300,     -- max time before re-queuing
    retries      INTEGER DEFAULT 0,
    max_retries  INTEGER DEFAULT 3
);
```

This is a database table, not new infrastructure. The coordinator writes to it. Workers read from it. The data protocol handles the transport.

### 2. Group-Aware Dispatch

A Lua API that lets a coordinator peer fan out function calls to group members:

```lua
-- Fan out to all online members in a group
local results = goop.group.fan_out("compute-group", "process_chunk", {
    chunks = split_data(dataset, member_count)
})
-- results = { [peer_id] = result, ... }

-- Call a specific peer
local result = goop.group.call(peer_id, "process_chunk", { data = chunk })

-- Queue work for offline peers (executes when they come online)
goop.group.enqueue("compute-group", "process_chunk", { data = chunk })
```

Under the hood, `fan_out` iterates online group members and makes parallel `lua-call` requests via the data protocol. `enqueue` writes to the work queue table.

### 3. Claim-and-Report Worker Loop

A background process on each worker peer that:

1. Periodically polls the coordinator for unclaimed work items (or receives push via group protocol)
2. Claims a work item (atomic UPDATE with peer ID)
3. Calls the local Lua function with the work item's parameters
4. Reports the result back to the coordinator
5. Marks the item complete (or failed with error)

```
Worker peer                              Coordinator peer
    |                                         |
    |  "Any work for me?"                     |
    |  lua-call: claim_work({group: "g1"})    |
    | --------------------------------------> |
    |                                         | SELECT from _work_queue
    |                                         | WHERE status='pending'
    |                                         | UPDATE status='claimed'
    |  {id: 42, function: "crunch",           |
    |   params: {chunk: [...]}}               |
    | <-------------------------------------- |
    |                                         |
    | (executes "crunch" locally)             |
    |                                         |
    |  lua-call: report_result({              |
    |    id: 42, result: {sum: 12345}})       |
    | --------------------------------------> |
    |                                         | UPDATE status='completed'
    |                                         | SET result=...
```

This is the lazy execution model. Work items sit in the queue until a peer comes online and claims them. No always-on workers required.

---

## Execution Models

### Immediate Fan-Out

The coordinator splits work and calls all online peers in parallel. Best for real-time computation where all workers are expected to be online.

```
Coordinator
    ├── lua-call ──> Peer A (chunk 1) ──> result A
    ├── lua-call ──> Peer B (chunk 2) ──> result B
    └── lua-call ──> Peer C (chunk 3) ──> result C

    aggregate(result A, result B, result C) -> final result
```

Use case: distributed search, parallel data processing, real-time aggregation.

### Queued (Lazy) Execution

The coordinator posts work items. Peers claim and execute them whenever they come online. The coordinator aggregates results as they arrive.

```
Time 0: Coordinator posts 10 work items

Time 1: Peer A comes online, claims items 1-3, executes, reports
Time 2: Peer B comes online, claims items 4-6, executes, reports
Time 3: Peer A claims items 7-8 (still online), executes, reports
Time 4: Peer C comes online, claims items 9-10, executes, reports

Time 5: All items complete, coordinator aggregates
```

Use case: batch processing, background computation, tasks that aren't time-sensitive.

### MapReduce

A combination of fan-out and aggregation. The coordinator distributes a map function, collects intermediate results, then distributes a reduce function.

```
Phase 1 (Map):
    Coordinator splits dataset into N chunks
    Each peer processes its chunk: map(chunk) -> intermediate

Phase 2 (Reduce):
    Coordinator collects intermediates
    Coordinator (or a peer) runs: reduce(intermediates) -> final
```

Use case: word counting across distributed text, statistical aggregation, index building.

---

## Concrete Example: Distributed Text Analysis

A community of literature enthusiasts wants to analyze word frequency across 100 books. Each peer has some books in their database.

**Coordinator script** (`site/lua/functions/analyze.lua`):

```lua
--- Coordinate distributed word frequency analysis
function call(request)
    -- Each peer in the group runs count_words locally
    -- on whatever books they have in their database
    local results = goop.group.fan_out("literature-club", "count_words", {
        pattern = request.params.pattern or ".*"
    })

    -- Aggregate word counts from all peers
    local totals = {}
    for peer_id, result in pairs(results) do
        if result.counts then
            for word, count in pairs(result.counts) do
                totals[word] = (totals[word] or 0) + count
            end
        end
    end

    return { word_counts = totals, peers_contributed = table_length(results) }
end
```

**Worker script** (on each peer, `site/lua/functions/count_words.lua`):

```lua
--- Count word frequencies in local book collection
function call(request)
    local rows = goop.db.query(
        "SELECT content FROM books WHERE title LIKE ?",
        request.params.pattern
    )

    local counts = {}
    for _, row in ipairs(rows) do
        for word in string.gmatch(string.lower(row.content), "%w+") do
            counts[word] = (counts[word] or 0) + 1
        end
    end

    return { counts = counts }
end
```

Each peer processes only its own data. No book content leaves any peer's machine. The coordinator receives only aggregated word counts. Privacy is structural, not policy-based.

---

## The Energy Argument

A typical cloud compute job:

1. Developer pushes code to CI/CD
2. Cloud provider provisions a VM (or wakes a container)
3. VM runs on a server in a data center drawing 500-1500W
4. Data center cooling adds 30-50% overhead (PUE ~1.3-1.5)
5. Network infrastructure routes the traffic
6. Result is returned, VM idles or is deprovisioned
7. Total energy: non-trivial, paid for by the developer, externalized to the grid

The same job on Goop2:

1. Coordinator posts work items
2. Peers that are already running (laptop open, desktop on) claim work
3. Lua VM starts in ~50 microseconds, uses negligible additional CPU
4. SQLite query runs against local data already on disk
5. Result is sent over existing libp2p connection
6. No additional hardware powered on. No cooling overhead. No provisioning.
7. Total additional energy: close to zero

This only works because the computation is distributed across devices with excess capacity. It's not a replacement for high-performance computing — you won't train neural networks this way. But for the long tail of computation that doesn't need GPUs or terabytes of RAM — data aggregation, text processing, validation, scoring, indexing — consumer hardware is more than sufficient.

The world has billions of devices sitting idle. Using 0.1% of their capacity for useful work is more efficient than building another data center.

---

## Security Considerations

Distributed compute amplifies existing security concerns:

| Concern | Mitigation |
|---|---|
| Malicious work items | Workers only execute functions they've installed locally. The coordinator specifies a function name, not arbitrary code. |
| Result tampering | Coordinator can cross-validate by sending the same work to multiple peers and comparing results (Byzantine fault tolerance). |
| Resource exhaustion | Existing per-invocation limits: 5s timeout, 10MB memory, 3 HTTP requests. Workers can set their own rate limits. |
| Data leakage | Each peer processes its own local data. Intermediate results should be aggregated, not raw data. |
| Coordinator abuse | Workers opt in to groups voluntarily. A worker can leave a group or ignore its queue at any time. |
| Free-riding | Peers that only consume computation without contributing can be tracked via the work queue. The group host can enforce minimum participation. |

The key principle: **workers never execute code they didn't choose to install**. A coordinator says "call function X with parameters Y." If the worker doesn't have function X, it declines. The coordinator cannot inject arbitrary computation — only invoke pre-installed, sandboxed Lua functions.

---

## What This Is Not

This is not a competitor to AWS Lambda or Azure Functions. Those services offer:

- Guaranteed uptime and SLAs
- Millisecond cold-start at global scale
- Integration with managed databases, queues, and storage
- Pay-per-invocation billing with no capacity planning

Goop2 distributed compute offers none of that. What it offers instead:

- **Zero infrastructure cost** — no servers, no billing, no accounts
- **Zero operational overhead** — no deployment, no monitoring, no scaling
- **Structural privacy** — data stays on the peer that owns it
- **Resilience through redundancy** — losing a peer doesn't lose the system
- **Energy efficiency** — uses idle capacity instead of dedicated hardware

It's useful for communities that want to compute together without renting cloud infrastructure. A research group aggregating results. A classroom running distributed experiments. A game community computing leaderboards. A sensor network processing readings.

Small-scale, community-owned, ephemeral computation. Not enterprise. Not production. Just peers helping peers.

---

## Implementation Path

### Phase 1: Fan-Out Primitive

Add `goop.group.fan_out(group_id, function, params)` to the Lua API. This iterates online group members, makes parallel `lua-call` requests, and collects results. No work queue, no persistence — purely synchronous. Useful immediately for real-time distributed queries.

Requires:
- Group membership list accessible from Lua (already available via peer table + group protocol)
- Parallel `lua-call` dispatch from within a Lua function (new capability — the engine currently can't make outbound data protocol calls)
- Result collection and timeout handling

### Phase 2: Work Queue

Add the `_work_queue` table and two coordinator-side Lua functions: `claim_work` and `report_result`. Workers poll for available work. No new protocol — everything runs over existing `lua-call`.

Requires:
- `goop.db.exec` for writes from Lua functions (implemented)
- Worker-side polling loop (could be a Lua script on a timer, or a Go-side background goroutine)
- TTL-based re-queuing of stale claimed items

### Phase 3: Push Notifications

Replace polling with push. When the coordinator adds work items, it notifies online group members via the group protocol. Workers that receive a notification immediately claim and execute, without the polling delay.

Requires:
- Group protocol integration with the Lua engine
- Event-driven worker activation

### Phase 4: MapReduce Abstraction

Build a higher-level API on top of fan-out and the work queue that handles the map-shuffle-reduce pattern. Probably a Lua library (prefab) rather than a Go-level primitive.

---

## Closing Thought

The original internet was designed as a network of equals that could survive nuclear war by routing around damage. Somewhere along the way, we centralized everything into a handful of data centers and called it "the cloud."

Goop2's distributed compute doesn't try to replace the cloud. It asks a simpler question: for the computation that doesn't need five-nines uptime and petabyte storage — the homework, the community project, the local analysis, the just-for-fun experiment — do we really need to rent a server?

Every peer is already a computer. Let them compute.
