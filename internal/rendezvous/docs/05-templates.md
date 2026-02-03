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

When you create a site from a template, Goop2 copies the template files into your `site/` directory and initializes the database schema.

## Built-in templates

### Blog

A personal blog where visitors can read posts and leave comments. Only the site owner can create posts.

- **Category**: Content
- **Insert policy**: `owner` for posts, `email` for comments

### Enquete

A simple survey application. Visitors answer questions and responses are collected in the owner's database.

- **Category**: Community
- **Insert policy**: `email` for responses

### Tic-Tac-Toe

A multiplayer tic-tac-toe game. Visitors can challenge each other or play against the computer. Server-side Lua enforces the game rules.

- **Category**: Games
- **Insert policy**: `open` for game creation

## Template store

If a rendezvous server has a `templates_dir` configured, it acts as a template store. Peers connected to that rendezvous server can browse and download templates from the store.

Templates appear on the rendezvous server's public page under "Template Store". Each template shows its name, category, description, and a download link.

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
4. Optionally add `schema.sql` for database tables.
5. Optionally add Lua functions in `lua/functions/` for server-side logic.

Your template will appear in the viewer's template browser and can be shared via the template store.
