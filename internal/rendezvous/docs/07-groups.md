# Groups & Collaboration

Groups are real-time, multi-peer communication channels. They enable features like multiplayer games, live quizzes, collaborative editing, and group chat.

## How groups work

Groups use a **host-relayed model**. The peer that creates a group acts as the hub. Every member opens a long-lived bidirectional stream to the host, and the host relays messages between members.

```
Member A  <--stream-->  Host  <--stream-->  Member B
                         |
                    (fan-out relay)
```

This model works naturally because the host already serves the site, stores data, and knows its visitors.

## Group types

| Type | Lifecycle | Visibility | Use case |
|------|-----------|------------|----------|
| **Ephemeral** | Auto-dissolves when activity ends | Activity-scoped | Games, quizzes |
| **Persistent** | Exists until explicitly closed | Ongoing | Study groups, teams |
| **Open** | Any peer can join | Listed in UI | Community chat |
| **Invite-only** | Requires invitation | Private | Private projects |

## Creating a group

Groups are created by the host peer, typically through a template's UI. The group is stored in the host's SQLite database and made available to visitors.

## Joining a group

Members join by opening a stream to the host on the `/goop/group/1.0.0` protocol. The join flow is:

1. Member sends `{"type":"join","group":"<group-id>"}`.
2. Host validates and adds the member.
3. Host sends a `welcome` message with the current member list and state.
4. Host broadcasts an updated `members` list to all other members.

## Message types

| Type | Direction | Purpose |
|------|-----------|---------|
| `join` | Member to Host | Request to join a group |
| `welcome` | Host to Member | Confirmation with current state |
| `members` | Host to Members | Updated member list |
| `msg` | Both directions | Application message (chat, game move) |
| `state` | Host to Member | Full state sync (for reconnection) |
| `leave` | Member to Host | Member leaving |
| `close` | Host to Members | Group is being closed |
| `error` | Host to Member | Error response |

## JavaScript API

Templates interact with groups through the `Goop.group` API:

```javascript
// List available groups
const groups = await Goop.data.query("_groups");

// Join a group
Goop.group.join("chess-42", function(msg) {
    console.log("Received:", msg);
});

// Send a message
Goop.group.send({ move: "e2e4" });

// Leave the group
Goop.group.leave();
```

## Use cases

### Multiplayer game

1. Host creates an ephemeral group for a chess match.
2. Opponent joins via the game UI.
3. Moves are exchanged in real time through the group channel.
4. Move history is stored in the host's database.
5. When the game ends, the group dissolves but the history persists.

### Live quiz

1. Teacher creates an open group for a quiz session.
2. Students join the group.
3. Timer synchronization and leaderboard updates flow through the group.
4. Answers are stored in the teacher's database for grading.

### Study group

1. Host creates a persistent group.
2. Members join for real-time discussion.
3. Shared notes and resources are stored in the host's database.
4. The group persists across sessions until the host closes it.

## Interaction with other protocols

| Task | Protocol |
|------|----------|
| Serve the UI | `/goop/site/1.0.0` |
| Store persistent data | `/goop/data/1.0.0` |
| Real-time messaging | `/goop/group/1.0.0` |
| Discover groups | Query `_groups` via `/goop/data/1.0.0` |
