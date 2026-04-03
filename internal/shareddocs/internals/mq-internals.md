# MQ Internals

## Protocol

<!-- STUB: Wire format, ACK-based delivery -->
<!-- Protocol ID: /goop/mq/1.0.0 -->
<!-- Message envelope: type, from, topic, payload, msg_id -->

## Topics

<!-- STUB: Topic naming conventions -->
<!-- group:{groupID}:{type} — group events (join, members, msg, close, etc.) -->
<!-- group.invite — group invitations -->
<!-- peer:announce — peer coming online -->
<!-- peer:gone — peer going offline -->
<!-- listen:{groupID}:state — listen player state updates -->
<!-- chat:{peerID} — direct chat messages -->

## SSE endpoints

<!-- STUB: Only two SSE endpoints in the system -->
<!-- /api/mq/events — MQ bus (all real-time events to the browser) -->
<!-- /api/logs/stream — log tailing -->

## Local vs remote

<!-- STUB: PublishLocal vs Send -->
<!-- PublishLocal — delivers to local MQ subscribers (same process, no P2P) -->
<!-- Send — delivers to a remote peer over P2P with ACK -->

## Encryption

<!-- STUB: Optional NaCl encryption layer for MQ payloads -->
