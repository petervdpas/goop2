Feature: Group subscription lifecycle
  As a peer in the goop2 network
  I want my group subscriptions to be tracked correctly
  So that I can see which groups I have joined

  Background:
    Given a fresh database

  # ── Invite flow ─────────────────────────────────────────────────────────

  Scenario: Receiving an invite creates a subscription
    When I receive an invite for group "g1" named "Blog Co-authors" of type "template" from host "host-abc"
    Then I should have 1 subscription
    And subscription "g1" should have name "Blog Co-authors"
    And subscription "g1" should have host "host-abc"
    And subscription "g1" should not be volatile

  Scenario: Multiple invites from different hosts create separate subscriptions
    When I receive an invite for group "g1" named "Blog" of type "template" from host "host-a"
    And I receive an invite for group "g2" named "Kanban" of type "template" from host "host-b"
    Then I should have 2 subscriptions

  Scenario: Duplicate invite for same group overwrites subscription
    When I receive an invite for group "g1" named "Old Name" of type "template" from host "host-a"
    And I receive an invite for group "g1" named "New Name" of type "template" from host "host-a"
    Then I should have 1 subscription
    And subscription "g1" should have name "New Name"

  # ── Non-volatile preservation ───────────────────────────────────────────

  Scenario: Non-volatile invite does not wipe existing subscriptions
    Given I have a subscription to group "g-existing" named "Kanban" of type "template" from host "host-a"
    When I receive an invite for group "g-new" named "Blog" of type "template" from host "host-b"
    Then I should have 2 subscriptions

  # ── Volatile wipe ──────────────────────────────────────────────────────

  Scenario: Volatile invite wipes stale subscriptions of the same type
    Given a registered volatile type "cluster"
    And I have a subscription to group "g-old" named "Old Cluster" of type "cluster" from host "host-a"
    When I receive an invite for group "g-new" named "New Cluster" of type "cluster" from host "host-b"
    Then I should have 1 subscription
    And subscription "g-new" should have name "New Cluster"

  Scenario: Volatile invite does not wipe subscriptions of a different type
    Given a registered volatile type "cluster"
    And I have a subscription to group "g-template" named "Blog" of type "template" from host "host-a"
    When I receive an invite for group "g-cluster" named "Compute" of type "cluster" from host "host-b"
    Then I should have 2 subscriptions

  # ── Host closes group ──────────────────────────────────────────────────

  Scenario: Host closing a group removes the client subscription
    Given I have a subscription to group "g1" named "Blog" of type "template" from host "host-abc"
    And I have an active connection to group "g1" hosted by "host-abc" of type "template"
    When the host closes group "g1"
    Then I should have 0 subscriptions

  # ── Subscription survives failed join ──────────────────────────────────

  Scenario: Subscription persists when auto-join fails
    When I receive an invite for group "g1" named "Blog" of type "template" from host "host-abc"
    Then I should have 1 subscription

  # ── Host-side member management ────────────────────────────────────────

  Scenario: Remote peer joining adds them to the member list
    Given I host a group "g1" named "Test" of type "template"
    And I have joined my own group "g1"
    When remote peer "peer-a" joins group "g1"
    Then group "g1" should have 2 members

  Scenario: Remote peer leaving removes them from the member list
    Given I host a group "g1" named "Test" of type "template"
    And I have joined my own group "g1"
    And remote peer "peer-a" joins group "g1"
    When remote peer "peer-a" leaves group "g1"
    Then group "g1" should have 1 member

  Scenario: Full group rejects new members
    Given I host a group "g1" named "Test" of type "template" with max 2 members
    And I have joined my own group "g1"
    And remote peer "peer-a" joins group "g1"
    When remote peer "peer-b" joins group "g1"
    Then group "g1" should have 2 members
