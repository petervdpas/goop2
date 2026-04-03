# Architecture

## System overview

Two repos: `goop2` (gateway, `:8787`) and `goop2-services` (microservices). Zero imports between them — communication is purely HTTP.

<!-- STUB: The peer lifecycle: startup, discovery, connection, serving -->

<!-- STUB: Package structure map: what lives where -->
<!-- internal/p2p — libp2p node, protocols, peer discovery -->
<!-- internal/lua — Lua sandbox engine, goop.* API -->
<!-- internal/viewer — HTTP server, routes, UI templates -->
<!-- internal/group — Group manager, TypeHandler, message routing -->
<!-- internal/storage — SQLite database, system tables, ORM persistence -->
<!-- internal/mq — Message queue protocol, SSE, topic routing -->
<!-- internal/orm — Schema validation, access enforcement, query building -->
<!-- internal/sdk — JavaScript SDK served to template viewers -->
<!-- internal/group_types — TypeHandler implementations per group type -->
<!-- internal/sitetemplates — Built-in embedded templates (blog, clubhouse, etc.) -->
<!-- internal/rendezvous — Rendezvous server, docs site, credit proxy -->
<!-- internal/app/modes — Peer and rendezvous startup modes -->
