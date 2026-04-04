Feature: Clubhouse template wiring
  As a developer maintaining the clubhouse template
  I want to verify all SDK dependencies and MQ wiring are correct
  So that the template works without runtime errors

  # ── SDK file availability ──────────────────────────────────────────────

  Scenario Outline: Clubhouse SDK scripts are served
    When I request SDK file "<file>"
    Then the SDK response status should be 200
    And the SDK content type should be "application/javascript"

    Examples:
      | file                       |
      | goop-data.js               |
      | goop-identity.js           |
      | goop-mq.js                 |
      | goop-chatroom.js           |
      | goop-component-base.js     |
      | goop-component-toast.js    |
      | goop-component-dialog.js   |
      | goop-component-template.js |

  Scenario Outline: Clubhouse SDK stylesheets are served
    When I request SDK file "<file>"
    Then the SDK response status should be 200
    And the SDK content type should be "text/css"

    Examples:
      | file                       |
      | goop-component-toast.css   |
      | goop-component-dialog.css  |

  Scenario: Nonexistent SDK file returns 404
    When I request SDK file "goop-nope.js"
    Then the SDK response status should be 404

  # ── SDK MQ subscribe contract ──────────────────────────────────────────

  Scenario: SDK MQ subscribe accepts topic and callback
    Given the SDK MQ is loaded
    Then Goop.mq.subscribe should accept a topic and callback

  Scenario: SDK MQ topic wildcard matches chat room prefix
    Given the SDK MQ is loaded
    Then topic "chat.room:*" should match "chat.room:abc123:msg"
    And topic "chat.room:*" should match "chat.room:abc123:members"
    And topic "chat.room:*" should match "chat.room:abc123:history"
    And topic "chat.room:*" should not match "group:abc123:msg"
    And topic "chat.room:*" should not match "chat:something"

  # ── Chatroom subscribe parses topics correctly ─────────────────────────

  Scenario: Chatroom subscribe extracts groupId and action from topic
    Given the chatroom topic parser is loaded
    When I parse topic "chat.room:abc123:msg"
    Then the parsed group ID should be "abc123"
    And the parsed action should be "msg"

  Scenario: Chatroom subscribe handles compound group IDs
    Given the chatroom topic parser is loaded
    When I parse topic "chat.room:18a3196cf3b90284:history"
    Then the parsed group ID should be "18a3196cf3b90284"
    And the parsed action should be "history"

  Scenario: Chatroom subscribe ignores malformed topics
    Given the chatroom topic parser is loaded
    When I parse topic "chat.room:noaction"
    Then the parsed topic should be invalid

  # ── Clubhouse manifest ─────────────────────────────────────────────────

  Scenario: Clubhouse manifest declares rooms schema
    Given the clubhouse manifest
    Then the manifest name should be "Clubhouse"
    And the manifest schemas should contain "rooms"

  Scenario: Rooms schema has required columns
    Given the rooms schema
    Then the schema should have column "name"
    And the schema should have column "description"
    And the schema should have column "group_id"
    And the schema should have column "max_members"
    And the schema should have column "status"
