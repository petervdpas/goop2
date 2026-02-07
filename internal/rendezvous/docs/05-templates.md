# Templates

Templates are pre-built applications that turn a Goop2 peer into a specific kind of site -- a blog, a quiz, a game, or anything else. Each template bundles HTML, CSS, JavaScript, a database schema, and optional Lua logic into a package that works out of the box.

## How templates work

A template lives in a directory with the following structure:

```
templates/
  my-template/
    manifest.json      # Metadata (name, description, icon, category)
    index.html         # Frontend UI (Go template syntax)
    style.css          # Template-specific styles
    app.js             # Client-side logic
    schema.sql         # SQLite schema (tables, indexes)
    lua/functions/     # Server-side Lua logic (optional)
      my-function.lua
```

When you apply a template, Goop2 copies the template files into your `site/` directory and initializes the database schema. You can do this from the viewer's **Templates** page.

## Built-in templates

These ship with the Goop2 binary and are always available:

### Blog

A personal blog where visitors can read posts and leave comments. Only the site owner can create posts.

- **Category**: Content
- **Insert policy**: `owner` for posts, `email` for comments

### Enquete

A simple survey application. Visitors answer questions and responses are collected in the owner's database.

- **Category**: Community
- **Insert policy**: `email` for responses

### Tic-Tac-Toe

A multiplayer tic-tac-toe game with server-side Lua enforcing the rules. Visitors can challenge the host to a match.

- **Category**: Games
- **Insert policy**: `open` for game creation

### Clubhouse

A real-time group chat room. Uses the groups protocol for live messaging between peers.

- **Category**: Community

## Store templates

Additional templates are available through the **template store** on a rendezvous server. These can be browsed and installed from the viewer's Templates page or from the rendezvous server's `/store` page.

Current store templates include:

| Template | Category | Description |
|----------|----------|-------------|
| **Chess** | Games | Classic chess with server-side move validation |
| **Quiz** | Education | Timed quizzes with scoring and leaderboard |
| **Corkboard** | Community | Pin notes and ads -- a digital bulletin board |
| **Kanban** | Productivity | Task board with drag-and-drop columns |
| **Photobook** | Content | Photo gallery with albums |
| **Arcade** | Games | Retro arcade games |

### Store API

The rendezvous server exposes a REST API for template management:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/store` | GET | Template store web page |
| `/api/templates` | GET | JSON list of available templates |
| `/api/templates/<name>/manifest.json` | GET | Template metadata |
| `/api/templates/<name>/bundle` | GET | Download template as tar.gz |

## Insert policies

Templates define who can write data to each table:

| Policy | Who can insert |
|--------|---------------|
| `owner` | Only the site owner. |
| `email` | Any peer with a verified email. |
| `open` | Any peer. |

The policy is defined per table in the template manifest:

```json
{
  "tables": {
    "posts": { "insert_policy": "owner" },
    "comments": { "insert_policy": "email" },
    "games": { "insert_policy": "open" }
  }
}
```

## Remote data

Templates work the same whether a visitor is viewing the site locally or remotely. The `goop-data.js` client library detects the context and routes data operations accordingly:

- **Local** (`/site/index.html`): Data operations go directly to the local database.
- **Remote** (`/p/<peerID>/index.html`): Data operations are proxied over a P2P stream to the remote peer's database.

The same template code handles both cases transparently.

## Creating a custom template

1. Create a directory in your templates folder (e.g. `templates/my-app/`).
2. Add a `manifest.json` with metadata and schema.
3. Write your `index.html`, `style.css`, and `app.js`.
4. Include `<script src="/assets/js/goop-data.js"></script>` for database access.
5. Optionally add `schema.sql` for database tables.
6. Optionally add Lua functions in `lua/functions/` for server-side logic.

Your template will appear in the viewer's template browser and can be shared via the template store.
