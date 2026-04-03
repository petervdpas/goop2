# P2P Networking

## Protocols

<!-- STUB: libp2p protocols and their IDs -->
<!-- /goop/site/1.0.0 — site content serving -->
<!-- /goop/data/1.0.0 — data proxy (ORM operations over P2P) -->
<!-- /goop/docs/1.0.0 — shared document listing and retrieval -->
<!-- /goop/mq/1.0.0 — message queue (ACK-based messaging) -->
<!-- /goop/listen/1.0.0 — audio streaming binary protocol -->

## Data proxy

<!-- STUB: How remote ORM/query calls route through P2P DataRequest -->
<!-- DataRequest struct: Op, Table, ID, Data, Where, Args, Order, Fields, Expr, GroupBy, KeyCol -->
<!-- Operations: query, insert, update, delete, query-one, exists, count, pluck, distinct, aggregate, update-where, delete-where, upsert, role -->
<!-- Access enforcement happens here (checkGroupAccess) -->

## Peer discovery

<!-- STUB: mDNS for LAN, rendezvous server for WAN -->
<!-- mdns_tag in config determines discovery group -->
<!-- Rendezvous server runs on Pi behind Caddy (goop2.com) -->

## NAT traversal

<!-- STUB: Circuit relay v2 + DCUtR hole-punching -->
<!-- How reachability works: SetReachable on first discovery, not on every heartbeat -->
