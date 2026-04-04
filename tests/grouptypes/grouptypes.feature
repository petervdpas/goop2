Feature: Group type handler lifecycle
  As a developer maintaining goop2 group types
  I want to verify each handler implements the TypeHandler contract correctly
  So that groups of every type behave predictably

  # ── chat ───────────────────────────────────────────────────────────────

  Scenario: Chat handler flags allow host to join
    Given a "chat" handler
    Then the handler should allow host join
    And the handler should not be volatile

  Scenario: Chat create and close lifecycle
    Given a "chat" handler
    When I create group "r1" named "Lobby" with max 0
    Then the handler should track 1 group
    When the handler closes group "r1"
    Then the handler should track 0 groups

  Scenario: Chat send and state
    Given a "chat" handler
    And I create group "r1" named "Lobby" with max 0
    When I send chat "hello" from "self-peer-id" to group "r1"
    Then group "r1" should have 1 chat message
    And the latest chat message in "r1" should be "hello"

  Scenario: Chat send to unknown room fails
    Given a "chat" handler
    When I send chat "oops" from "self-peer-id" to group "nonexistent"
    Then the chat send should fail

  # ── template ───────────────────────────────────────────────────────────

  Scenario: Template handler flags allow host to join
    Given a "template" handler
    Then the handler should allow host join
    And the handler should not be volatile

  Scenario: Template create and close lifecycle
    Given a "template" handler
    When I create group "t1" named "Blog" via the group manager with type "template"
    Then the group manager should list 1 hosted group
    When I close group "t1" via the group manager
    Then the group manager should list 0 hosted groups

  Scenario: Template group tracks members
    Given a "template" handler
    And I create group "t1" named "Blog" via the group manager with type "template"
    And I join my own group "t1"
    When remote peer "peer-a" joins group "t1"
    Then group "t1" should have 2 members via the group manager
    When remote peer "peer-a" leaves group "t1"
    Then group "t1" should have 1 member via the group manager

  # ── data-federation ────────────────────────────────────────────────────

  Scenario: Data federation handler flags allow host to join
    Given a "datafed" handler
    Then the handler should allow host join
    And the handler should not be volatile

  Scenario: Data federation create and close lifecycle
    Given a "datafed" handler
    When I create datafed group "d1" named "Federation"
    Then the datafed handler should track 1 group
    When the datafed handler closes group "d1"
    Then the datafed handler should track 0 groups

  Scenario: Data federation peer leave removes contribution
    Given a "datafed" handler
    And I create datafed group "d1" named "Federation"
    And peer "peer-b" contributes table "items" to datafed group "d1"
    When peer "peer-b" leaves datafed group "d1"
    Then datafed group "d1" should have 0 contributions

  # ── files ──────────────────────────────────────────────────────────────

  Scenario: Files handler flags allow host to join
    Given a "files" handler
    Then the handler should allow host join
    And the handler should not be volatile

  Scenario: Files handler registers with group manager
    Given a "files" handler
    When I create group "f1" named "Docs" via the group manager with type "files"
    Then the group manager should list 1 hosted group
    And the group flags for "f1" should allow host join

  # ── cluster ────────────────────────────────────────────────────────────

  Scenario: Cluster handler flags are volatile and deny host join
    Given a "cluster" handler
    Then the handler should not allow host join
    And the handler should be volatile

  Scenario: Cluster manager create and leave
    Given a cluster manager
    When I create cluster "c1"
    Then the cluster role should be "host"
    When I leave the cluster
    Then the cluster role should be ""

  Scenario: Cluster manager join as worker
    Given a cluster manager
    When I join cluster "c1" hosted by "host-peer"
    Then the cluster role should be "worker"

  Scenario: Cluster manager rejects double join
    Given a cluster manager
    And I create cluster "c1"
    When I join cluster "c2" hosted by "other-host"
    Then the join should fail

  Scenario: Cluster job submission requires host role
    Given a cluster manager
    When I submit a job of type "render"
    Then the submit should fail

  Scenario: Cluster host can submit and list jobs
    Given a cluster manager
    And I create cluster "c1"
    When I submit a job of type "render" with priority 5
    Then there should be 1 job
    And the job stats should show 1 pending

  # ── listen ─────────────────────────────────────────────────────────────

  Scenario: Stream URL detection
    Then "http://radio.example.com/stream" should be a stream URL
    And "https://live.example.com/audio" should be a stream URL
    And "/home/user/music.mp3" should not be a stream URL
    And "" should not be a stream URL
