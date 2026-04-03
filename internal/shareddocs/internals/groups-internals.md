# Group System Internals

## Manager

<!-- STUB: internal/group/manager.go -->
<!-- In-memory state: groups map[string]*hostedGroup, activeConns map[string]*clientConn -->
<!-- DB persistence: _groups, _group_members tables -->
<!-- MQ wiring: subscribes to group:, group.invite, peer:announce topics -->
<!-- Restored from DB on startup: existing groups loaded into memory -->

## TypeHandler interface

<!-- STUB: internal/group/typehandler.go -->
<!-- Flags() TypeFlags — HostCanJoin bool -->
<!-- OnCreate(groupID, name, maxMembers, volatile) error -->
<!-- OnJoin(groupID, peerID, isHost) -->
<!-- OnLeave(groupID, peerID, isHost) -->
<!-- OnClose(groupID) -->
<!-- OnEvent(evt *Event) -->
<!-- Registered via manager.RegisterType(groupType, handler) -->
<!-- TypeFlagsForGroup returns default flags if no handler registered -->

## Group type implementations

<!-- STUB: internal/group_types/ -->
<!-- template/ — handler.go (lifecycle), schema.go (AnalyzeSchemas), apply.go (Apply: create/reuse/close) -->
<!-- files/ — handler.go (lifecycle), store.go (Save/Read/Delete/List, 50MB limit, sha256 hash) -->
<!-- listen/ — manager.go (audio state), events.go (lifecycle + control messages), queue.go (playlist persistence), host.go (streaming), client.go, stream.go -->
<!-- cluster/ — manager.go, handler.go, dispatcher.go, worker.go, queue.go (job queue with priority), exec.go, types.go, messages.go -->
<!-- datafed/ — handler.go (lifecycle), contributions.go (peer contributions, AllPeerSources), sync.go (schema offer/withdraw/sync), peers.go (gone/announce suspend/restore) -->

## Message routing

<!-- STUB: Host-relayed model -->
<!-- broadcastToGroup sends to all members except sender -->
<!-- SendControl wraps payload with group_type key for type-specific dispatch -->
<!-- ExtractControl/ParseControl extract type-specific payloads from generic messages -->

## Member management

<!-- STUB: -->
<!-- Members stored in hostedGroup.members map[string]*memberMeta -->
<!-- memberMeta: peerID, role, joinedAt -->
<!-- Default role on join comes from hostedGroup.info.DefaultRole -->
<!-- Roles persisted in _group_members table -->
<!-- SetMemberRole: updates in-memory + DB + broadcasts updated member list -->

## Client-side (joining remote groups)

<!-- STUB: -->
<!-- activeConns: outbound connections to remote group hosts -->
<!-- Subscriptions persisted in _group_subscriptions for reconnection -->
<!-- reconnectSubscriptions runs on startup to rejoin previously connected groups -->
<!-- handleMemberMessage processes members, close, meta, ping, msg, state events -->

## Groups are never auto-deleted

<!-- STUB: -->
<!-- Only the owner removes groups (via Close or template switch) -->
<!-- Template apply closes groups where group_type == "template" AND group_context == old template name -->
<!-- No PurgeInvalid or startup cleanup exists -->
