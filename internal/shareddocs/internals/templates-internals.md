# Template System Internals

## Built-in templates

`internal/sitetemplates/` — embedded via `//go:embed`:

| Template | Category | Features |
| -- | -- | -- |
| `blog` | Content | Lua + JS, coauthor role |
| `clubhouse` | Community | Lua groups + JS MQ chat rooms |
| `tictactoe` | Game | Lua only |
| `todo` | Productivity | Lua + roles |
| `enquete` | Forms | require_email |

## Manifest format

Each template has a `manifest.json`:

```json
{
  "name": "Blog",
  "description": "A simple blogging platform",
  "category": "content",
  "icon": "📝",
  "schemas": ["posts"],
  "require_email": false,
  "default_role": "viewer",
  "tables": {}
}
```

- `schemas[]` — ORM table names owned by this template (used for cleanup)
- `tables{}` — legacy `map[string]TablePolicy` (insert_policy per table)
- `require_email` — viewers must have a verified email
- `default_role` — role assigned to new group members

## TemplateMeta struct

`internal/sitetemplates/embed.go`:

```go
type TemplateMeta struct {
    Name, Description, Category, Icon, Dir string
    Tables       map[string]TablePolicy
    Schemas      []string
    RequireEmail bool
    DefaultRole  string
}
```

## Template files

`SiteFiles(dir)` returns all files except `manifest.json` and `schema.sql`. These are copied to the peer's site directory on apply.

Typical template structure:
```
blog/
  manifest.json
  README.md          ← (planned) author-editable docs, stored in _meta not site dir
  schemas/posts.json
  index.html
  js/app.js
  css/style.css
  lua/functions/blog.lua
  lua/seed.lua
```

**Excluded from site copy** (handled separately during apply):
- `manifest.json` — parsed into `TemplateMeta`, stored in `_meta["template_manifest"]`
- `schema.sql` — executed against DB (legacy path)
- `README.md` — (planned) stored in `_meta["template_readme"]`, editable in viewer UI

## Apply flow

`internal/group_types/template/apply.go` — `Handler.Apply(ApplyConfig)`:

1. Check if existing group matches this template → reuse group
2. If switching templates: close ALL groups owned by old template (`closeTemplateGroups`)
3. Clear `template_group_id` and `template_group_name` from `_meta`
4. Analyze schemas for group access needs (`SchemaInfo.NeedsGroup`)
5. If group needed: create new group, auto-join host, configure roles
6. Store new group ID + template name in `_meta`

The full apply is orchestrated in `viewer/routes/templates.go`:

1. Copy site files to peer's site directory
2. Write schema JSON files to `<peerDir>/schemas/`
3. For each schema: `DeleteTable` then `CreateTableORM` (failover safety)
4. Store manifest in `_meta["template_manifest"]`
5. Track template-owned tables in `_meta["template_tables"]`
6. Call `Handler.Apply()` for group management
7. Call `EnsureLua` to start/rescan Lua engine
8. If `seed.lua` exists: call `seed` function via LuaCall

## Schema analysis

`internal/group_types/template/schema.go` — `AnalyzeSchemas`:

- Parses all `schemas/*.json` files
- `NeedsGroup` = true if any schema has group-level access
- `Roles` = union of all role names across all schemas
- Returns `SchemaInfo` used by `Apply`

## Template cleanup

On template switch:

- `dropTemplateTables` removes tables listed in `_meta["template_tables"]`
- `closeTemplateGroups` closes groups where `group_context == old template name`
- `CloseByContext` on chat manager closes chat rooms for the old template
- Schema files in `<peerDir>/schemas/` are overwritten by the new template

## Remote template store

When `templates_url` is configured:

- `RemoteTemplatesProvider` proxies to the templates microservice
- Bundle download: `GET /api/templates/{dir}/bundle` → .tar.gz
- Price/access checks go through credits service

When `templates_url` is empty:

- `LocalTemplateStore` reads templates from `presence.templates_dir`
