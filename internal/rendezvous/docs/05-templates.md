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

When you apply a template, Goop2 copies the template files into your `site/` directory and initializes the database schema. You can do this from the viewer's **Create > Templates** page.

## Built-in templates

These are embedded in the Goop2 binary and are always available:

### Blog

A personal blog where you and invited co-authors can write posts. Visitors see a read-only feed.

- **Category**: Content
- **Insert policy**: `group` for posts, `owner` for blog config

### Enquete

A simple survey application. Visitors answer questions and responses are collected in the owner's database.

- **Category**: Community
- **Insert policy**: `email` for responses

### Tic-Tac-Toe

A multiplayer tic-tac-toe game with server-side Lua enforcing the rules. Visitors can challenge the host or play against the computer.

- **Category**: Games
- **Insert policy**: `open` for game creation

### Clubhouse

A real-time group chat room. Uses the groups protocol for live messaging between peers.

- **Category**: Community

## Store templates

Additional templates are available through the **template store** on a rendezvous server. Store templates are served by the **templates microservice** -- a separate service from the [goop2-services](https://github.com/petervdpas/goop2-services) repository. The rendezvous server proxies requests to it when `templates_url` is configured with `use_services` enabled.

These can be browsed and installed from the viewer's Create > Templates page or from the rendezvous server's `/store` page.

Current store templates include:

| Template | Category | Description |
|----------|----------|-------------|
| **Quiz** | Community | A multiple-choice quiz with server-side scoring |
| **Space Invaders** | Games | Classic arcade shooter -- defend Earth from alien invaders |
| **Chess** | Games | Classic chess -- challenge visitors or play against the computer |
| **Corkboard** | Community | Community advertisement board -- pin notes, pushpins, color-coded cards |
| **Day Trader** | Finance | Real-time price charts for any trading instrument with configurable symbols and API |
| **Kanban** | Productivity | A shared team kanban board -- owner manages columns, co-authors collaborate on cards |
| **Photobook** | Content | A personal photo gallery with albums and lightbox |

When a credits service is also configured, store templates can be priced. Peers need sufficient credits to download priced templates.

### Local template store

If you don't want to run the templates microservice, you can serve templates directly from a local directory by setting `templates_dir` in your rendezvous config:

```json
{
  "presence": {
    "templates_dir": "templates"
  }
}
```

Each subdirectory needs a `manifest.json`. This fallback is used when `templates_url` is empty or `use_services` is false. Local templates are always free.

### Store API

The rendezvous server proxies these endpoints to the templates service (or serves them from the local directory):

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/store` | GET | Template store web page |
| `/api/templates` | GET | JSON list of available templates |
| `/api/templates/<name>/manifest` | GET | Template metadata |
| `/api/templates/<name>/bundle` | GET | Download template as tar.gz |

## Insert policies

Templates define who can write data to each table:

| Policy | Who can insert |
|--------|---------------|
| `owner` | Only the site owner. |
| `email` | Any peer with a verified email. |
| `group` | Members of an authorized group (e.g. blog co-authors). |
| `open` | Any peer. |

The policy is defined per table in the template manifest:

```json
{
  "tables": {
    "posts": { "insert_policy": "group" },
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

To create a template for the store, create a directory in the templates service's `templates_dir`:

1. Add a `manifest.json` with metadata and schema.
2. Write your `index.html`, `style.css`, and `app.js`.
3. Include `<script src="/sdk/goop-data.js"></script>` for database access.
4. Optionally add `schema.sql` for database tables.
5. Optionally add Lua functions in `lua/functions/` for server-side logic.

The template will appear in the store and can be installed by any peer connected to that rendezvous server. The templates service supports `extra_dirs` in its config for loading templates from multiple directories.
