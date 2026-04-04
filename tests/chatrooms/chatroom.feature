Feature: Chat room lifecycle
  As a peer hosting a site with the clubhouse template
  I want to create, use, and close chat rooms via the HTTP API
  So that visitors can chat in real time

  Background:
    Given a running chat room server

  # ── Room creation ──────────────────────────────────────────────────────

  Scenario: Creating a room returns room details
    When I create a room named "Lobby" with description "Welcome" and max members 10
    Then the response status should be 200
    And the response should contain room name "Lobby"

  Scenario: Creating a room without a name fails
    When I create a room named "" with description "" and max members 0
    Then the response status should be 400

  # ── Room state ─────────────────────────────────────────────────────────

  Scenario: New room state has no messages
    Given I have created a room named "Quiet Room"
    When I request the state of that room
    Then the response status should be 200
    And the state should have 0 messages

  # ── Sending messages ───────────────────────────────────────────────────

  Scenario: Sending a message to a room
    Given I have created a room named "Chat"
    When I send message "hello world" to that room
    Then the response status should be 200
    When I request the state of that room
    Then the state should have 1 message
    And the latest message text should be "hello world"

  Scenario: Sending multiple messages preserves order
    Given I have created a room named "Busy Room"
    When I send message "first" to that room
    And I send message "second" to that room
    And I send message "third" to that room
    And I request the state of that room
    Then the state should have 3 messages
    And the latest message text should be "third"

  Scenario: Sending to a nonexistent room fails
    When I send message "hello" to room "does-not-exist"
    Then the response status should be 500

  # ── Closing a room ─────────────────────────────────────────────────────

  Scenario: Closing a room removes it
    Given I have created a room named "Temporary"
    When I close that room
    Then the response status should be 200
    When I request the state of that room
    Then the response status should be 404

  # ── Validation ─────────────────────────────────────────────────────────

  Scenario: Send requires both group_id and text
    Given I have created a room named "Validate"
    When I send message "" to that room
    Then the response status should be 400
