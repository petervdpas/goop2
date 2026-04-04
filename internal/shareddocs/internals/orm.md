# ORM & Schema System

## Schema JSON format

Schema files live in `schemas/*.json` within a template. Structure:

```json
{
  "name": "posts",
  "system_key": true,
  "columns": [
    {"name": "title", "type": "text", "required": true},
    {"name": "body", "type": "text"},
    {"name": "status", "type": "text", "default": "draft"}
  ],
  "access": {
    "read": "open",
    "insert": "group",
    "update": "group",
    "delete": "owner"
  },
  "roles": {
    "coauthor": {"read": true, "insert": true, "update": true, "delete": false},
    "viewer": {"read": true, "insert": false, "update": false, "delete": false}
  }
}
```

### Column fields

| Field | Type | Purpose |
| -- | -- | -- |
| `name` | string | Column name |
| `type` | string | `text`, `integer`, `real`, `blob`, `guid`, `datetime`, `date`, `time` |
| `required` | bool | Reject inserts without this column |
| `default` | any | Default value for inserts |
| `key` | bool | Part of the primary key |
| `auto` | bool | Auto-increment (integer type) or auto-generate (guid, datetime, date, time) |

### system_key

When `system_key: true`, the table automatically gets:

- `_id` — auto-increment integer primary key
- `_owner` — peer ID of the row creator
- `_created_at` — timestamp of creation
- `_updated_at` — timestamp of last update

## Access policies

Four levels, defined per operation (read, insert, update, delete):

| Level | Who can access |
| -- | -- |
| `local` | Only the local node process (no remote access) |
| `owner` | Only the site owner (callerID == selfID) |
| `group` | Group members, checked against schema roles map |
| `open` | Any peer |

`UsesGroup()` returns true if any of read/insert/update/delete is set to `"group"`.

## Role-based access

`schema.RoleCanDo(roles, role, op)` checks whether the given role permits the operation.

- Schema is THE ACL — host enforces, client tries
- Owner always has full access regardless of roles map
- Unknown roles are denied
- Roles map keys are role names (e.g., "coauthor", "viewer")
- Values are `{read, insert, update, delete}` booleans

## Schema validation

- `ValidateInsert`: checks required columns are present, validates types and constraints
- `ValidateUpdate`: checks column existence and type compatibility
- Validation runs in the storage layer before SQL execution

## Data proxy operations

Full list of operations that go through the P2P data protocol (`/goop/data/1.0.0`):

| Op | Description | Remote peers |
| -- | -- | -- |
| `query` | Find rows with where, order, limit, offset | Yes (access-checked) |
| `query-one` | Find single row | Local only |
| `insert` | Insert row (with auto-column generation) | Yes (access-checked) |
| `update` | Update row by ID | Yes (access-checked) |
| `delete` | Delete row by ID | Yes (access-checked) |
| `exists` | Check if rows matching condition exist | Local only |
| `count` | Count rows matching condition | Local only |
| `pluck` | Extract single column values | Local only |
| `distinct` | Distinct values for a column | Local only |
| `aggregate` | SQL aggregate expression (SUM, AVG, etc.) | Local only |
| `update-where` | Bulk update with condition | Local only |
| `delete-where` | Bulk delete with condition | Local only |
| `upsert` | Insert or update by key column | Local only |
| `role` | Query caller's role and permissions for a table | Yes |
| `tables` | List all tables | Yes |
| `orm-schema` | List all ORM schemas | Yes |
| `describe` | Table column info | Yes |
| `lua-call` | Invoke a Lua data function | Yes |
| `lua-list` | List available Lua data functions | Yes |
| `create-table`, `add-column`, `drop-column`, `rename-table`, `delete-table` | Schema operations | Local only |
