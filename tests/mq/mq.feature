Feature: MQ message bus
  As the central event bus connecting all subsystems
  I want reliable topic routing, payload fidelity, and correct identity propagation
  So that chat, groups, calls, and presence all behave consistently

  Background:
    Given a fresh MQ dispatcher

  # ═══════════════════════════════════════════════════════════════════════
  # Core dispatch mechanics
  # ═══════════════════════════════════════════════════════════════════════

  # ── Topic routing ────────────────────────────────────────────────────

  Scenario: Exact topic match dispatches to subscriber
    Given a subscriber on prefix "chat"
    When a message arrives on topic "chat" from "peer-A"
    Then the "chat" subscriber should have received 1 message

  Scenario: Prefix match dispatches to subscriber
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1:offer" from "peer-A"
    Then the "call:" subscriber should have received 1 message

  Scenario: Non-matching topic does not dispatch
    Given a subscriber on prefix "call:"
    When a message arrives on topic "chat" from "peer-A"
    Then the "call:" subscriber should have received 0 messages

  Scenario: Multiple subscribers on different prefixes
    Given a subscriber on prefix "chat"
    And a subscriber on prefix "group:"
    When a message arrives on topic "chat" from "peer-A"
    And a message arrives on topic "group:g1:join" from "peer-B"
    Then the "chat" subscriber should have received 1 message
    And the "group:" subscriber should have received 1 message

  Scenario: Unsubscribed callback no longer fires
    Given a subscriber on prefix "chat"
    When the "chat" subscriber is unsubscribed
    And a message arrives on topic "chat" from "peer-A"
    Then the "chat" subscriber should have received 0 messages

  # ── SSE delivery vs suppression ──────────────────────────────────────

  Scenario: Direct chat is delivered to SSE listeners
    Given an SSE listener
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"hello"}
    Then the SSE listener should have received 1 event
    And the SSE event topic should be "chat"
    And the SSE event from should be "peer-A"

  Scenario: Chat broadcast is delivered to SSE listeners
    Given an SSE listener
    When a message arrives on topic "chat.broadcast" from "peer-A" with payload {"content":"yo"}
    Then the SSE listener should have received 1 event

  Scenario: Group protocol messages are suppressed from SSE
    Given an SSE listener
    When a message arrives on topic "group:g1:join" from "peer-A"
    Then the SSE listener should have received 0 events

  Scenario: Group invite is suppressed from SSE
    Given an SSE listener
    When a message arrives on topic "group.invite" from "peer-A"
    Then the SSE listener should have received 0 events

  Scenario: Chat room messages are suppressed from SSE
    Given an SSE listener
    When a message arrives on topic "chat.room:g1:msg" from "peer-A"
    Then the SSE listener should have received 0 events

  Scenario: Call signals are delivered to SSE listeners
    Given an SSE listener
    When a message arrives on topic "call:ch1:offer" from "peer-A" with payload {"type":"call-offer","sdp":"v=0"}
    Then the SSE listener should have received 1 event
    And the SSE event topic should be "call:ch1:offer"

  Scenario: Unknown topics are delivered to SSE listeners
    Given an SSE listener
    When a message arrives on topic "custom:something" from "peer-A" with payload "data"
    Then the SSE listener should have received 1 event

  # ── Inbox buffering and replay ───────────────────────────────────────

  Scenario: Messages buffer when no SSE listener is connected
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"buffered"}
    And an SSE listener connects
    Then the SSE listener should have received 1 event
    And the SSE event payload "content" should be "buffered"

  Scenario: Buffered messages from multiple peers are replayed
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"from-A"}
    And a message arrives on topic "chat" from "peer-B" with payload {"content":"from-B"}
    And an SSE listener connects
    Then the SSE listener should have received 2 events

  Scenario: Inbox is cleared after replay
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"first"}
    And an SSE listener connects
    And the SSE listener disconnects
    And a second SSE listener connects
    Then the second SSE listener should have received 0 events

  Scenario: Inbox cap drops oldest messages
    When 210 messages arrive on topic "chat" from "peer-A"
    And an SSE listener connects
    Then the SSE listener should have received 200 events

  # ── PublishLocal ─────────────────────────────────────────────────────

  Scenario: PublishLocal delivers to SSE listener
    Given an SSE listener
    When a local event is published on topic "peer:announce" with payload {"peerID":"p1","content":"Alice"}
    Then the SSE listener should have received 1 event
    And the SSE event topic should be "peer:announce"

  Scenario: PublishLocal does not trigger topic subscribers
    Given a subscriber on prefix "peer:"
    And an SSE listener
    When a local event is published on topic "peer:announce" with payload {"peerID":"p1"}
    Then the "peer:" subscriber should have received 0 messages
    And the SSE listener should have received 1 event

  # ── NotifyDelivered ──────────────────────────────────────────────────

  Scenario: NotifyDelivered sends delivered event to SSE listener
    Given an SSE listener
    When delivery is confirmed for message "msg-42"
    Then the SSE listener should have received 1 event
    And the SSE event type should be "delivered"
    And the SSE event msg_id should be "msg-42"

  # ── Application-level ACK (mq.ack topic) ─────────────────────────────

  Scenario: mq.ack topic triggers delivered event and stops propagation
    Given an SSE listener
    And a subscriber on prefix "mq"
    When a message arrives on topic "mq.ack" from "peer-A" with payload {"msg_id":"orig-1"}
    Then the SSE listener should have received 1 event
    And the SSE event type should be "delivered"
    And the SSE event msg_id should be "orig-1"
    And the "mq" subscriber should have received 0 messages

  Scenario: mq.ack with empty msg_id is silently dropped
    Given an SSE listener
    When a message arrives on topic "mq.ack" from "peer-A" with payload {"msg_id":""}
    Then the SSE listener should have received 0 events

  # ═══════════════════════════════════════════════════════════════════════
  # Payload fidelity
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: String payload arrives intact
    Given a subscriber on prefix "chat"
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"hello world"}
    Then the "chat" subscriber payload should have "content" = "hello world"

  Scenario: Nested object payload arrives intact
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:msg" from "peer-A" with payload {"data":{"key":"val","num":42}}
    Then the "group:" subscriber payload should have nested "data.key" = "val"

  Scenario: Array payload arrives intact
    Given a subscriber on prefix "custom:"
    When a message arrives on topic "custom:test" from "peer-A" with payload {"items":["a","b","c"]}
    Then the "custom:" subscriber payload should have array "items" with 3 elements

  Scenario: Boolean and null payload fields arrive intact
    Given a subscriber on prefix "test:"
    When a message arrives on topic "test:bools" from "peer-A" with payload {"flag":true,"empty":null}
    Then the "test:" subscriber payload should have "flag" = true
    And the "test:" subscriber payload "empty" should be null

  # ═══════════════════════════════════════════════════════════════════════
  # Identity propagation
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Subscriber receives correct sender peer ID
    Given a subscriber on prefix "chat"
    When a message arrives on topic "chat" from "peer-sender-123"
    Then the "chat" subscriber from should be "peer-sender-123"

  Scenario: SSE event carries correct sender peer ID
    Given an SSE listener
    When a message arrives on topic "chat" from "peer-sender-456" with payload {"content":"hi"}
    Then the SSE event from should be "peer-sender-456"

  Scenario: Different senders on same topic are distinguishable
    Given a subscriber on prefix "chat"
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"from A"}
    And a message arrives on topic "chat" from "peer-B" with payload {"content":"from B"}
    Then the "chat" subscriber should have received 2 messages
    And the "chat" subscriber message 1 from should be "peer-A"
    And the "chat" subscriber message 2 from should be "peer-B"

  # ═══════════════════════════════════════════════════════════════════════
  # Direct chat subsystem
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Direct chat message dispatches to chat subscriber only
    Given a subscriber on prefix "chat"
    And a subscriber on prefix "group:"
    And a subscriber on prefix "call:"
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"dm"}
    Then the "chat" subscriber should have received 1 message
    And the "group:" subscriber should have received 0 messages
    And the "call:" subscriber should have received 0 messages

  Scenario: Chat broadcast dispatches to chat subscriber
    Given a subscriber on prefix "chat"
    When a message arrives on topic "chat.broadcast" from "peer-A" with payload {"content":"all"}
    Then the "chat" subscriber should have received 1 message

  Scenario: Chat topic does not match chat.room prefix
    Given a subscriber on prefix "chat.room:"
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"dm"}
    Then the "chat.room:" subscriber should have received 0 messages

  # ═══════════════════════════════════════════════════════════════════════
  # Chat room subsystem
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Chat room message dispatches to chat.room subscriber
    Given a subscriber on prefix "chat.room:"
    When a message arrives on topic "chat.room:g1:msg" from "peer-A" with payload {"action":"msg","message":{"id":"m1","from":"peer-A","from_name":"Alice","text":"hi","timestamp":1000}}
    Then the "chat.room:" subscriber should have received 1 message
    And the "chat.room:" subscriber payload should have "action" = "msg"

  Scenario: Chat room history dispatches to chat.room subscriber
    Given a subscriber on prefix "chat.room:"
    When a message arrives on topic "chat.room:g1:history" from "peer-A" with payload {"action":"history","messages":[]}
    Then the "chat.room:" subscriber should have received 1 message

  Scenario: Chat room members dispatches to chat.room subscriber
    Given a subscriber on prefix "chat.room:"
    When a message arrives on topic "chat.room:g1:members" from "peer-A" with payload {"action":"members","members":[{"peer_id":"p1","name":"Alice"}]}
    Then the "chat.room:" subscriber should have received 1 message

  Scenario: Chat room subtopics are distinguishable
    Given a subscriber on prefix "chat.room:"
    When a message arrives on topic "chat.room:g1:msg" from "peer-A"
    And a message arrives on topic "chat.room:g1:history" from "peer-A"
    And a message arrives on topic "chat.room:g1:members" from "peer-A"
    Then the "chat.room:" subscriber should have received 3 messages
    And the "chat.room:" subscriber message 1 topic should be "chat.room:g1:msg"
    And the "chat.room:" subscriber message 2 topic should be "chat.room:g1:history"
    And the "chat.room:" subscriber message 3 topic should be "chat.room:g1:members"

  # ═══════════════════════════════════════════════════════════════════════
  # Group protocol subsystem
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Group join dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:join" from "peer-A"
    Then the "group:" subscriber should have received 1 message

  Scenario: Group welcome dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:welcome" from "host-peer" with payload {"group_name":"Room","group_type":"chat","max_members":10,"volatile":false,"members":[]}
    Then the "group:" subscriber should have received 1 message

  Scenario: Group members update dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:members" from "host-peer" with payload {"members":[{"peer_id":"p1","name":"Alice","role":"owner"}]}
    Then the "group:" subscriber should have received 1 message

  Scenario: Group msg dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:msg" from "peer-A" with payload {"data":"custom"}
    Then the "group:" subscriber should have received 1 message

  Scenario: Group state dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:state" from "host-peer" with payload {"state":"active"}
    Then the "group:" subscriber should have received 1 message

  Scenario: Group leave dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:leave" from "peer-A"
    Then the "group:" subscriber should have received 1 message

  Scenario: Group close dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:close" from "host-peer"
    Then the "group:" subscriber should have received 1 message

  Scenario: Group error dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:error" from "host-peer" with payload {"code":"full","message":"group is full"}
    Then the "group:" subscriber should have received 1 message
    And the "group:" subscriber payload should have "code" = "full"

  Scenario: Group ping dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:ping" from "host-peer"
    Then the "group:" subscriber should have received 1 message

  Scenario: Group pong dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:pong" from "peer-A"
    Then the "group:" subscriber should have received 1 message

  Scenario: Group meta dispatches to group subscriber
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:g1:meta" from "host-peer" with payload {"group_name":"Renamed","group_type":"chat","max_members":20}
    Then the "group:" subscriber should have received 1 message

  Scenario: Group invite dispatches to group.invite subscriber
    Given a subscriber on prefix "group.invite"
    When a message arrives on topic "group.invite" from "host-peer" with payload {"group_id":"g1","group_name":"Room","host_peer_id":"host-peer","group_type":"chat"}
    Then the "group.invite" subscriber should have received 1 message
    And the "group.invite" subscriber payload should have "group_name" = "Room"

  Scenario: All group message types are suppressed from SSE
    Given an SSE listener
    When a message arrives on topic "group:g1:join" from "peer-A"
    And a message arrives on topic "group:g1:welcome" from "host-peer"
    And a message arrives on topic "group:g1:members" from "host-peer"
    And a message arrives on topic "group:g1:msg" from "peer-A"
    And a message arrives on topic "group:g1:state" from "host-peer"
    And a message arrives on topic "group:g1:leave" from "peer-A"
    And a message arrives on topic "group:g1:close" from "host-peer"
    And a message arrives on topic "group:g1:error" from "host-peer"
    And a message arrives on topic "group:g1:ping" from "host-peer"
    And a message arrives on topic "group:g1:pong" from "peer-A"
    And a message arrives on topic "group:g1:meta" from "host-peer"
    And a message arrives on topic "group.invite" from "host-peer"
    Then the SSE listener should have received 0 events

  # ── Group topic parsing ──────────────────────────────────────────────

  Scenario: Group topic extracts groupID and message type
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:abc123:join" from "peer-A"
    Then the "group:" subscriber topic should be "group:abc123:join"

  Scenario: Group prefix does not match group.invite
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group.invite" from "host-peer"
    Then the "group:" subscriber should have received 0 messages

  # ═══════════════════════════════════════════════════════════════════════
  # Call signaling subsystem
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Call request dispatches to call subscriber
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1" from "caller-peer" with payload {"type":"call-request"}
    Then the "call:" subscriber should have received 1 message
    And the "call:" subscriber payload should have "type" = "call-request"

  Scenario: Call ack dispatches to call subscriber
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1" from "callee-peer" with payload {"type":"call-ack"}
    Then the "call:" subscriber should have received 1 message

  Scenario: Call offer with SDP dispatches to call subscriber
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1" from "caller-peer" with payload {"type":"call-offer","sdp":"v=0\r\no=- 123 IN IP4 0.0.0.0"}
    Then the "call:" subscriber should have received 1 message
    And the "call:" subscriber payload should have "sdp" = "v=0\r\no=- 123 IN IP4 0.0.0.0"

  Scenario: Call answer dispatches to call subscriber
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1" from "callee-peer" with payload {"type":"call-answer","sdp":"v=0\r\na=answer"}
    Then the "call:" subscriber should have received 1 message

  Scenario: ICE candidate dispatches to call subscriber
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1" from "peer-A" with payload {"type":"ice-candidate","candidate":{"candidate":"a]candidate:1 1 udp 2113937151 192.168.1.1 5000 typ host","sdpMid":"0","sdpMLineIndex":0}}
    Then the "call:" subscriber should have received 1 message

  Scenario: Call hangup dispatches to call subscriber
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1" from "peer-A" with payload {"type":"call-hangup","channel_id":"ch1"}
    Then the "call:" subscriber should have received 1 message

  Scenario: Full call signaling sequence dispatches in order
    Given a subscriber on prefix "call:"
    When a message arrives on topic "call:ch1" from "caller" with payload {"type":"call-request"}
    And a message arrives on topic "call:ch1" from "callee" with payload {"type":"call-ack"}
    And a message arrives on topic "call:ch1" from "caller" with payload {"type":"call-offer","sdp":"offer-sdp"}
    And a message arrives on topic "call:ch1" from "callee" with payload {"type":"call-answer","sdp":"answer-sdp"}
    And a message arrives on topic "call:ch1" from "caller" with payload {"type":"ice-candidate","candidate":{"candidate":"c1","sdpMid":"0","sdpMLineIndex":0}}
    And a message arrives on topic "call:ch1" from "callee" with payload {"type":"call-hangup","channel_id":"ch1"}
    Then the "call:" subscriber should have received 6 messages
    And the "call:" subscriber message 1 from should be "caller"
    And the "call:" subscriber message 6 from should be "callee"

  Scenario: Call signals are delivered to SSE (not suppressed)
    Given an SSE listener
    When a message arrives on topic "call:ch1" from "peer-A" with payload {"type":"call-request"}
    Then the SSE listener should have received 1 event

  Scenario: Loopback ICE publishes locally to SSE
    Given an SSE listener
    When a local event is published on topic "call:loopback:ch1" with payload {"type":"loopback-ice","channel_id":"ch1","candidate":{"candidate":"c1","sdpMid":"0","sdpMLineIndex":0}}
    Then the SSE listener should have received 1 event
    And the SSE event topic should be "call:loopback:ch1"

  # ═══════════════════════════════════════════════════════════════════════
  # Peer presence subsystem
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Peer announce publishes to SSE listener
    Given an SSE listener
    When a local event is published on topic "peer:announce" with payload {"peerID":"peer-1","content":"Alice","reachable":true,"offline":false,"lastSeen":1700000000000}
    Then the SSE listener should have received 1 event
    And the SSE event topic should be "peer:announce"

  Scenario: Peer announce payload fields arrive intact
    Given an SSE listener
    When a local event is published on topic "peer:announce" with payload {"peerID":"peer-1","content":"Alice","email":"a@b.com","avatarHash":"abc","videoDisabled":false,"activeTemplate":"blog","publicKey":"pk1","encryptionSupported":true,"verified":true,"goopClientVersion":"2.4.52","reachable":true,"offline":false,"lastSeen":1700000000000,"favorite":true}
    Then the SSE event payload "peerID" should be "peer-1"
    And the SSE event payload "content" should be "Alice"
    And the SSE event payload "goopClientVersion" should be "2.4.52"
    And the SSE event payload "encryptionSupported" should be true
    And the SSE event payload "reachable" should be true

  Scenario: Peer gone publishes to SSE listener
    Given an SSE listener
    When a local event is published on topic "peer:gone" with payload {"peerID":"peer-1"}
    Then the SSE listener should have received 1 event
    And the SSE event topic should be "peer:gone"

  Scenario: Peer announce does not leak to group or chat subscribers
    Given a subscriber on prefix "group:"
    And a subscriber on prefix "chat"
    And an SSE listener
    When a local event is published on topic "peer:announce" with payload {"peerID":"p1","content":"Alice"}
    Then the "group:" subscriber should have received 0 messages
    And the "chat" subscriber should have received 0 messages
    And the SSE listener should have received 1 event

  # ═══════════════════════════════════════════════════════════════════════
  # Peer identity subsystem
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Identity request dispatches to identity subscriber
    Given a subscriber on prefix "identity"
    When a message arrives on topic "identity" from "peer-A"
    Then the "identity" subscriber should have received 1 message
    And the "identity" subscriber from should be "peer-A"

  Scenario: Identity response dispatches to identity.response subscriber
    Given a subscriber on prefix "identity.response"
    When a message arrives on topic "identity.response" from "peer-B" with payload {"peerID":"peer-B","content":"Bob","email":"bob@test.com","avatarHash":"abc","goopClientVersion":"2.4.52"}
    Then the "identity.response" subscriber should have received 1 message
    And the "identity.response" subscriber payload should have "content" = "Bob"
    And the "identity.response" subscriber payload should have "goopClientVersion" = "2.4.52"

  Scenario: Identity request is suppressed from SSE
    Given an SSE listener
    When a message arrives on topic "identity" from "peer-A"
    Then the SSE listener should have received 0 events

  Scenario: Identity response is suppressed from SSE
    Given an SSE listener
    When a message arrives on topic "identity.response" from "peer-B" with payload {"peerID":"peer-B","content":"Bob"}
    Then the SSE listener should have received 0 events

  Scenario: Identity subscriber does not receive identity.response
    Given a subscriber on prefix "identity.response"
    When a message arrives on topic "identity" from "peer-A"
    Then the "identity.response" subscriber should have received 0 messages

  # ═══════════════════════════════════════════════════════════════════════
  # Cross-subsystem isolation
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: All subsystems receive only their own traffic
    Given a subscriber on prefix "chat"
    And a subscriber on prefix "chat.room:"
    And a subscriber on prefix "group:"
    And a subscriber on prefix "group.invite"
    And a subscriber on prefix "call:"
    And a subscriber on prefix "identity"
    And a subscriber on prefix "identity.response"
    And an SSE listener
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"dm"}
    And a message arrives on topic "chat.room:g1:msg" from "peer-B" with payload {"action":"msg"}
    And a message arrives on topic "group:g1:join" from "peer-C"
    And a message arrives on topic "group.invite" from "peer-D" with payload {"group_id":"g2"}
    And a message arrives on topic "call:ch1" from "peer-E" with payload {"type":"call-request"}
    And a message arrives on topic "identity" from "peer-F"
    And a message arrives on topic "identity.response" from "peer-G" with payload {"peerID":"peer-G","content":"Grace"}
    Then the "chat" subscriber should have received 1 message
    And the "chat.room:" subscriber should have received 1 message
    And the "group:" subscriber should have received 1 message
    And the "group.invite" subscriber should have received 1 message
    And the "call:" subscriber should have received 1 message
    And the "identity" subscriber should have received 1 message
    And the "identity.response" subscriber should have received 1 message
    And the SSE listener should have received 2 events

  Scenario: Chat prefix does not accidentally match chat.room or chat.broadcast
    Given a subscriber on prefix "chat.room:"
    When a message arrives on topic "chat" from "peer-A"
    And a message arrives on topic "chat.broadcast" from "peer-A"
    Then the "chat.room:" subscriber should have received 0 messages

  Scenario: Group prefix does not accidentally match group.invite
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group.invite" from "peer-A"
    Then the "group:" subscriber should have received 0 messages

  Scenario: Call prefix does not match chat or group
    Given a subscriber on prefix "call:"
    When a message arrives on topic "chat" from "peer-A"
    And a message arrives on topic "group:g1:msg" from "peer-A"
    Then the "call:" subscriber should have received 0 messages

  # ═══════════════════════════════════════════════════════════════════════
  # Concurrent and edge cases
  # ═══════════════════════════════════════════════════════════════════════

  Scenario: Multiple SSE listeners each receive the same event
    Given 3 SSE listeners
    When a message arrives on topic "chat" from "peer-A" with payload {"content":"multi"}
    Then all 3 SSE listeners should have received 1 event each

  Scenario: SSE listener cancel removes it cleanly
    Given an SSE listener
    When the SSE listener disconnects
    And a message arrives on topic "chat" from "peer-A" with payload {"content":"after-cancel"}
    Then no panic should have occurred

  Scenario: Double cancel does not panic
    Given an SSE listener
    When the SSE listener disconnects
    And the SSE listener disconnects again
    Then no panic should have occurred

  Scenario: Empty payload dispatches without error
    Given a subscriber on prefix "chat"
    When a message arrives on topic "chat" from "peer-A" with empty payload
    Then the "chat" subscriber should have received 1 message

  Scenario: Message with very long topic dispatches correctly
    Given a subscriber on prefix "group:"
    When a message arrives on topic "group:abcdef1234567890abcdef1234567890:msg" from "peer-A"
    Then the "group:" subscriber should have received 1 message

  Scenario: Log topics do not generate recursive log events
    Given an SSE listener
    When a message arrives on topic "log:mq" from "peer-A"
    Then the SSE listener should have received 1 event
