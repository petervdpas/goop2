# Storage Internals

## SQLite database

<!-- STUB: One SQLite DB per peer at <peerDir>/data.db -->

## System tables

<!-- STUB: _tables — user-created tables with insert_policy -->
<!-- _meta — key-value store for template settings, group IDs, etc. -->
<!-- _groups — hosted groups with id, name, owner, group_type, group_context, max_members, default_role, roles (JSON), volatile, host_joined -->
<!-- _group_members — group_id + peer_id + role -->
<!-- _group_subscriptions — remote groups this peer has joined -->
<!-- _orm_schemas — persisted ORM schema JSON per table -->

## Key _meta entries

<!-- STUB: -->
<!-- template_group_id — ID of the template's auto-created group -->
<!-- template_group_name — name of the active template -->
<!-- template_manifest — full manifest JSON of the active template -->
<!-- template_require_email — "1" if template requires email -->
<!-- template_tables — comma-separated list of template-owned table names -->

## ORM table creation

<!-- STUB: How schemas/*.json are parsed and turned into SQLite tables -->
<!-- CreateTableORM: creates table + stores schema in _orm_schemas -->
<!-- system_key: auto-increment _id, plus _owner, _created_at, _updated_at -->
<!-- Auto columns: guid, datetime, date, time, integer (auto-increment) -->

## Access enforcement

<!-- STUB: Access is NOT enforced in the storage layer -->
<!-- Enforcement happens in the P2P data proxy (internal/p2p/data.go) -->
<!-- Storage layer just reads/writes — caller is responsible for access checks -->
