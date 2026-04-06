Feature: ORM data operations
  As the core data API backing goop.orm() and db.orm()
  I want reliable CRUD, querying, and schema management
  So that templates can safely store and retrieve structured data

  Background:
    Given a fresh ORM database
    And an ORM table "tasks" with columns:
      | name     | type | key  | required | auto |
      | id       | guid | true | true     | true |
      | title    | text |      | true     |      |
      | status   | text |      |          |      |
      | priority | int  |      |          |      |
    And the "tasks" access policy is read="open" insert="open" update="owner" delete="owner"

  # ═══════════════════════════════════════════════════════════════════════
  # Schema management
  # ═��════════════���═══════════════════════════════════���════════════════════

  Scenario: Table appears in table list as ORM mode
    When I list all tables
    Then the table list should contain "tasks" with mode "orm"

  Scenario: Describe ORM table returns schema with columns
    When I describe table "tasks"
    Then the describe mode should be "orm"
    And the schema should have 4 columns

  Scenario: Create table with missing name is rejected
    When I create a table with name "" and columns:
      | name | type | key  | required | auto |
      | x    | text |      |          |      |
    Then the response status should be 400

  Scenario: Create table with missing columns is rejected
    When I create a table with name "empty" and no columns
    Then the response status should be 400

  Scenario: Delete table removes it from the list
    When I create a table with name "temp" and columns:
      | name | type | key  | required | auto |
      | id   | guid | true | true     | true |
      | val  | text |      |          |      |
    And I delete table "temp"
    And I list all tables
    Then the table list should not contain "temp"

  Scenario: Export schema returns table definition
    When I export schema for "tasks"
    Then the response status should be 200

  Scenario: List ORM schemas returns all schemas
    When I list ORM schemas
    Then the response status should be 200

  # ═════════════════════════════���═════════════════════════════════════════
  # Insert and basic retrieval
  # ═══���════��══════════════════════════════════════════════════════════════

  Scenario: Insert a row and retrieve it with find
    When I insert into "tasks" with data:
      """
      {"title": "Write tests", "status": "open", "priority": 1}
      """
    Then the insert should succeed with an id
    When I find all rows in "tasks"
    Then the result should have 1 row
    And row 1 field "title" should be "Write tests"
    And row 1 field "status" should be "open"

  Scenario: Insert sets system fields automatically
    When I insert into "tasks" with data:
      """
      {"title": "Auto fields"}
      """
    And I find all rows in "tasks"
    Then row 1 should have a non-empty "_owner" field
    And row 1 should have a non-empty "_created_at" field

  Scenario: Insert with missing required field fails
    When I insert into "tasks" with data:
      """
      {"status": "open"}
      """
    Then the response status should be 500

  Scenario: Multiple inserts produce correct count
    When I insert into "tasks" with data:
      """
      {"title": "First", "priority": 1}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Second", "priority": 2}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Third", "priority": 3}
      """
    And I find all rows in "tasks"
    Then the result should have 3 rows

  # ════��════════════════════════════════════════════════════��═════════════
  # Query operations
  # ════════════════════════════════════════════��══════════════════════════

  Scenario: Find with WHERE clause filters results
    When I insert into "tasks" with data:
      """
      {"title": "Alpha", "status": "open"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Beta", "status": "closed"}
      """
    And I find rows in "tasks" where "status = ?" with args:
      """
      ["open"]
      """
    Then the result should have 1 row
    And row 1 field "title" should be "Alpha"

  Scenario: Find with ORDER and LIMIT
    When I insert into "tasks" with data:
      """
      {"title": "C", "priority": 3}
      """
    And I insert into "tasks" with data:
      """
      {"title": "A", "priority": 1}
      """
    And I insert into "tasks" with data:
      """
      {"title": "B", "priority": 2}
      """
    And I find rows in "tasks" ordered by "priority ASC" limit 2
    Then the result should have 2 rows
    And row 1 field "title" should be "A"
    And row 2 field "title" should be "B"

  Scenario: Find with field selection
    When I insert into "tasks" with data:
      """
      {"title": "Selected", "status": "open", "priority": 5}
      """
    And I find rows in "tasks" selecting fields:
      """
      ["title", "priority"]
      """
    Then the result should have 1 row
    And row 1 field "title" should be "Selected"

  Scenario: Find-one returns single row
    When I insert into "tasks" with data:
      """
      {"title": "Only one", "status": "done"}
      """
    And I find one in "tasks" where "status = ?" with args:
      """
      ["done"]
      """
    Then the single result field "title" should be "Only one"

  Scenario: Find-one returns null when no match
    When I find one in "tasks" where "title = ?" with args:
      """
      ["nonexistent"]
      """
    Then the single result should be null

  Scenario: Get-by looks up by column value
    When I insert into "tasks" with data:
      """
      {"title": "Lookup target", "status": "active"}
      """
    And I get from "tasks" by column "status" value "active"
    Then the single result field "title" should be "Lookup target"

  Scenario: Get-by returns null when not found
    When I get from "tasks" by column "title" value "ghost"
    Then the single result should be null

  # ════���════════════════════════════════════════��═════════════════════════
  # Exists and count
  # ═══��════════���═════════════════════════════════════���════════════════════

  Scenario: Exists returns true for matching row
    When I insert into "tasks" with data:
      """
      {"title": "Findable"}
      """
    And I check exists in "tasks" where "title = ?" with args:
      """
      ["Findable"]
      """
    Then exists should be true

  Scenario: Exists returns false for no match
    When I check exists in "tasks" where "title = ?" with args:
      """
      ["Nope"]
      """
    Then exists should be false

  Scenario: Count returns correct number of rows
    When I insert into "tasks" with data:
      """
      {"title": "One"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Two"}
      """
    And I count rows in "tasks"
    Then the count should be 2

  Scenario: Count with WHERE clause
    When I insert into "tasks" with data:
      """
      {"title": "Open1", "status": "open"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Open2", "status": "open"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Closed1", "status": "closed"}
      """
    And I count rows in "tasks" where "status = ?" with args:
      """
      ["open"]
      """
    Then the count should be 2

  # ═══════════════════════════════════════════════════════════════��═══════
  # Pluck and distinct
  # ═���══════════════��══════════════════════════════��═══════════════════════

  Scenario: Pluck extracts single column as flat array
    When I insert into "tasks" with data:
      """
      {"title": "AAA", "status": "open"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "BBB", "status": "closed"}
      """
    And I pluck "title" from "tasks" ordered by "title ASC"
    Then the pluck result should be:
      """
      ["AAA", "BBB"]
      """

  Scenario: Distinct returns unique values
    When I insert into "tasks" with data:
      """
      {"title": "A", "status": "open"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "B", "status": "open"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "C", "status": "closed"}
      """
    And I get distinct "status" from "tasks"
    Then the distinct result should have 2 values

  # ═══���═══════════════════════════���═══════════════════════════════════════
  # Aggregate
  # ═════════════════���═════════════════════════════════════════════════════

  Scenario: Aggregate COUNT returns row count
    When I insert into "tasks" with data:
      """
      {"title": "X", "priority": 10}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Y", "priority": 20}
      """
    And I aggregate "COUNT(*) as n" on "tasks"
    Then the aggregate result should have 1 row
    And aggregate row 1 field "n" should be 2

  Scenario: Aggregate SUM computes total
    When I insert into "tasks" with data:
      """
      {"title": "A", "priority": 10}
      """
    And I insert into "tasks" with data:
      """
      {"title": "B", "priority": 20}
      """
    And I aggregate "SUM(priority) as total" on "tasks"
    Then aggregate row 1 field "total" should be 30

  Scenario: Aggregate with GROUP BY
    When I insert into "tasks" with data:
      """
      {"title": "A", "status": "open", "priority": 5}
      """
    And I insert into "tasks" with data:
      """
      {"title": "B", "status": "open", "priority": 3}
      """
    And I insert into "tasks" with data:
      """
      {"title": "C", "status": "closed", "priority": 7}
      """
    And I aggregate "COUNT(*) as n" on "tasks" grouped by "status"
    Then the aggregate result should have 2 rows

  # ════════════════════��════════════════════════════��═════════════════════
  # Update and delete operations
  # ════════════════��══════════════════════════════════════════════════════

  Scenario: Update row by id
    When I insert into "tasks" with data:
      """
      {"title": "Original", "status": "draft"}
      """
    And I update the inserted row in "tasks" with data:
      """
      {"status": "published"}
      """
    And I find all rows in "tasks"
    Then row 1 field "status" should be "published"

  Scenario: Delete row by id
    When I insert into "tasks" with data:
      """
      {"title": "Delete me"}
      """
    And I delete the inserted row from "tasks"
    And I find all rows in "tasks"
    Then the result should have 0 rows

  Scenario: Update-where modifies matching rows
    When I insert into "tasks" with data:
      """
      {"title": "A", "status": "draft"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "B", "status": "draft"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "C", "status": "done"}
      """
    And I update-where in "tasks" set data where "status = ?" with args:
      """
      {"data": {"status": "published"}, "args": ["draft"]}
      """
    Then the affected count should be 2

  Scenario: Delete-where removes matching rows
    When I insert into "tasks" with data:
      """
      {"title": "Keep", "status": "active"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Remove1", "status": "trash"}
      """
    And I insert into "tasks" with data:
      """
      {"title": "Remove2", "status": "trash"}
      """
    And I delete-where from "tasks" where "status = ?" with args:
      """
      ["trash"]
      """
    Then the affected count should be 2
    When I find all rows in "tasks"
    Then the result should have 1 row

  Scenario: Upsert updates existing row
    When I insert into "tasks" with data:
      """
      {"title": "Existing", "status": "v1"}
      """
    And I upsert in "tasks" with key_col "title" and data:
      """
      {"title": "Existing", "status": "v2"}
      """
    And I find one in "tasks" where "title = ?" with args:
      """
      ["Existing"]
      """
    Then the single result field "status" should be "v2"

  Scenario: Upsert inserts new row with auto-generated fields
    When I upsert in "tasks" with key_col "title" and data:
      """
      {"title": "Brand new", "status": "fresh"}
      """
    And I find all rows in "tasks"
    Then the result should have 1 row
    And row 1 field "title" should be "Brand new"
    And row 1 field "status" should be "fresh"

  Scenario: Upsert with missing key_col value is rejected
    When I upsert in "tasks" with key_col "title" and data:
      """
      {"status": "no-key"}
      """
    Then the response status should be 500

  # ═════���════════════════════════════════════════════════════���════════════
  # Access policies
  # ═════════════════════════════════════════════���═════════════════════════

  Scenario: Set insert policy to group
    When I set the insert policy of "tasks" to "group"
    Then the response status should be 200

  Scenario: Set insert policy to invalid value is rejected
    When I set the insert policy of "tasks" to "invalid"
    Then the response status should be 400

  Scenario: Role endpoint returns owner for table owner
    When I get my role for "tasks"
    Then my role should be "owner"

  # ═══════════════════════════════���═════════════════════════��═════════════
  # Table DDL operations
  # ��═══════════════���══════════════════════════════════════════════════════

  Scenario: Rename table updates the name
    When I create a table with name "old_name" and columns:
      | name | type | key  | required | auto |
      | id   | guid | true | true     | true |
      | val  | text |      |          |      |
    And I rename table "old_name" to "new_name"
    And I list all tables
    Then the table list should contain "new_name" with mode "orm"
    And the table list should not contain "old_name"

  # ═══════════════════════════════════════════════════════════���═══════════
  # Validation and error cases
  # ════════════════════════════════════════════════════���══════════════════

  Scenario: Insert with missing table name is rejected
    When I insert into "" with data:
      """
      {"title": "x"}
      """
    Then the response status should be 400

  Scenario: Find-one with missing table name is rejected
    When I find one in "" where "1=1" with args:
      """
      []
      """
    Then the response status should be 400

  Scenario: Count with missing table name is rejected
    When I count rows in ""
    Then the response status should be 400

  Scenario: Exists with missing table name is rejected
    When I check exists in "" where "1=1" with args:
      """
      []
      """
    Then the response status should be 400

  Scenario: Pluck with missing column is rejected
    When I pluck "" from "tasks" ordered by ""
    Then the response status should be 400

  Scenario: Distinct with missing column is rejected
    When I get distinct "" from "tasks"
    Then the response status should be 400

  Scenario: Aggregate with missing expr is rejected
    When I aggregate "" on "tasks"
    Then the response status should be 400

  Scenario: Update-where with missing where clause is rejected
    When I update-where in "tasks" set data where "" with args:
      """
      {"data": {"x": "y"}, "args": []}
      """
    Then the response status should be 400

  Scenario: Delete-where with missing where clause is rejected
    When I delete-where from "tasks" where "" with args:
      """
      []
      """
    Then the response status should be 400

  Scenario: Upsert with missing key_col is rejected
    When I upsert in "tasks" with key_col "" and data:
      """
      {"title": "x"}
      """
    Then the response status should be 400

  Scenario: Describe with missing table name is rejected
    When I describe table ""
    Then the response status should be 400
