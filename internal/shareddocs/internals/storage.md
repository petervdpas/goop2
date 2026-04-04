# Storage Internals

## SQLite database

One SQLite database per peer at `<peerDir>/data.db`. Opened via `storage.Open(configDir)`. All system tables are created on first open with `CREATE TABLE IF NOT EXISTS`. Schema migrations use `ALTER TABLE ADD COLUMN` with error suppression for idempotency.

## System tables

| Table | Primary key | Purpose |
| -- | -- | -- |
| `_meta` | `key TEXT` | Key-value store for template settings, group IDs, flags |
| `_tables` | `name TEXT` | Registry of user-created tables with `schema` JSON and `insert_policy` |
| `_orm_schemas` | `table_name TEXT` | Persisted ORM schema JSON per table |
| `_groups` | `id TEXT` | Hosted groups: name, owner, group_type, group_context, max_members, default_role, roles (JSON), volatile, host_joined |
| `_group_members` | `(group_id, peer_id)` | Group membership: peer_id + role per group |
| `_group_subscriptions` | `(host_peer_id, group_id)` | Remote groups this peer has joined: group_name, group_type, role, max_members, volatile, host_name |
| `_cluster_jobs` | `id TEXT` | Cluster compute jobs: type, mode, payload, priority, timeout, status, worker_id, result, progress |
| `_peer_cache` | `peer_id TEXT` | Full presence data cache: content, email, avatar_hash, video_disabled, active_template, verified, addrs, protocols, public_key |
| `_chat_messages` | `id INTEGER AUTOINCREMENT` | Direct chat history: peer_id, from_id, content, ts. Indexed by `(peer_id, ts DESC)` |
| `_favorites` | `peer_id TEXT` | Favorite peers with full metadata — never pruned by TTL |

## Key _meta entries

| Key | Value | Purpose |
| -- | -- | -- |
| `template_group_id` | Group ID string | ID of the template's auto-created group |
| `template_group_name` | Template dir name | Name of the active template |
| `template_manifest` | Full JSON | Complete manifest of the active template |
| `template_require_email` | `"1"` or absent | Template requires email for viewer access |
| `template_tables` | Comma-separated names | Template-owned table names (for cleanup on switch) |

## ORM table creation

When a template defines `schemas/*.json`:

1. Schema JSON is parsed: `name`, `system_key`, `columns[]`, `access{}`, `roles{}`
2. `CreateTableORM` creates the SQLite table + stores schema in `_orm_schemas`
3. When `system_key: true`: auto-creates `_id` (auto-increment primary key), `_owner`, `_created_at`, `_updated_at` columns
4. Auto columns by type: `guid` (UUID), `datetime`/`date`/`time` (current timestamp), `integer` with `auto: true` (auto-increment)
5. Schema files are written to `<peerDir>/schemas/`, not the `site/` content directory
6. `_meta["template_tables"]` tracks template-owned tables — `dropTemplateTables` only removes those

## Access enforcement

Access is NOT enforced in the storage layer. The storage layer reads and writes without access checks.

Enforcement happens in the P2P data proxy (`internal/p2p/data.go`):

- `checkGroupAccess` verifies the caller's group role permits the operation
- Schema is THE ACL — host enforces, client tries
- Owner (callerID == selfID) always has full access

## Table deletion

`DeleteTable` cleans both `_tables` and `_orm_schemas` entries for a given table name.
