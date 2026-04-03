# ORM & Schema System

## Schema JSON format

<!-- STUB: schemas/*.json structure -->
<!-- name, system_key, columns[], access{}, roles{} -->
<!-- Column fields: name, type, required, default, key, auto -->
<!-- Types: text, integer, real, blob, guid, datetime, date, time -->

## Access policies

<!-- STUB: Four levels -->
<!-- local — only the local node process -->
<!-- owner — only the site owner (peerID == selfID) -->
<!-- group — group members, checked against schema roles map -->
<!-- open — any peer -->
<!-- UsesGroup() returns true if any of read/insert/update/delete is "group" -->

## Role-based access

<!-- STUB: schema.RoleCanDo(roles, role, op) -->
<!-- Schema is THE ACL — host enforces, client tries -->
<!-- Owner always has full access regardless of roles map -->
<!-- Unknown roles are denied -->

## Schema validation

<!-- STUB: ValidateInsert checks required columns, types, constraints -->
<!-- ValidateUpdate checks column existence and types -->

## Data proxy operations

<!-- STUB: Full list of operations that go through P2P data protocol -->
<!-- query — find rows with where, order, limit, offset -->
<!-- query-one — find single row -->
<!-- insert — insert row (with auto-column generation) -->
<!-- update — update row by ID -->
<!-- delete — delete row by ID -->
<!-- exists — check if rows matching condition exist -->
<!-- count — count rows matching condition -->
<!-- pluck — extract single column values -->
<!-- distinct — distinct values for a column -->
<!-- aggregate — SQL aggregate expression (SUM, AVG, etc.) -->
<!-- update-where — bulk update with condition -->
<!-- delete-where — bulk delete with condition -->
<!-- upsert — insert or update -->
<!-- role — query caller's role and permissions for a table -->
<!-- tables — list all tables -->
<!-- schemas — list all ORM schemas -->
