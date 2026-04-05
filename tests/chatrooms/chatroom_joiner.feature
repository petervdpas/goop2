Feature: Chat room joiner perspective
  As a peer joining a remote chat room
  I want to see room members, send messages, and receive updates
  So that the chat experience works from both sides

  Background:
    Given a running host and joiner chat server

  # ── Joining a room ────────────────────────────────────────────────────

  Scenario: Joiner can retrieve room state after joining
    Given the host has created room "Lobby" with description "Welcome"
    And peer "joiner-peer" has joined the host group for "Lobby"
    When the joiner requests the state of "Lobby"
    Then the joiner response status should be 200
    And the joiner state should have room name "Lobby"

  Scenario: Joiner sees members after joining
    Given the host has created room "Lobby" with description "Welcome"
    And peer "joiner-peer" has joined the host group for "Lobby"
    When the joiner requests the state of "Lobby"
    Then the joiner state should have at least 1 member

  # ── Sending messages from joiner ──────────────────────────────────────

  Scenario: Joiner can send a message
    Given the host has created room "Chat" with description "Talk"
    And peer "joiner-peer" has joined the host group for "Chat"
    When the joiner sends message "hello from joiner" to "Chat"
    Then the joiner response status should be 200

  Scenario: Message from joiner appears in room state
    Given the host has created room "Chat" with description "Talk"
    And peer "joiner-peer" has joined the host group for "Chat"
    And the joiner sends message "hello from joiner" to "Chat"
    When the joiner requests the state of "Chat"
    Then the joiner state should have 1 message
    And the joiner latest message text should be "hello from joiner"

  # ── Member list updates ───────────────────────────────────────────────

  Scenario: Host sees joiner in member list
    Given the host has created room "Party" with description "Fun"
    And peer "joiner-peer" has joined the host group for "Party"
    When the host requests the state of "Party"
    Then the host state should have 2 members

  Scenario: After joiner leaves the host member list updates
    Given the host has created room "Party" with description "Fun"
    And peer "joiner-peer" has joined the host group for "Party"
    And peer "joiner-peer" has left the host group for "Party"
    When the host requests the state of "Party"
    Then the host state should have 1 member

  # ── Name resolution ──────────────────────────────────────────────────

  Scenario: Joiner sees real peer names in member list
    Given the host has created room "Names" with description "Test names"
    And peer "joiner-peer" has joined the host group for "Names"
    When the joiner requests the state of "Names"
    Then the joiner member "host-peer-id" should have name "Host"
    And the joiner member "joiner-peer" should have name "Joiner"

  Scenario: Message from joiner has resolved sender name
    Given the host has created room "Named" with description "Test"
    And peer "joiner-peer" has joined the host group for "Named"
    And the joiner sends message "hi there" to "Named"
    When the joiner requests the state of "Named"
    Then the joiner latest message from_name should be "Joiner"
