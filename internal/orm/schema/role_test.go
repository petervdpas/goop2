package schema

import "testing"

func TestRoleCanDoOwnerAlwaysAllowed(t *testing.T) {
	roles := map[string]RoleAccess{
		"writer": {Insert: true},
	}
	for _, op := range []string{"read", "insert", "update", "delete"} {
		if !RoleCanDo(roles, "owner", op) {
			t.Fatalf("owner should always be allowed to %s", op)
		}
	}
}

func TestRoleCanDoCustomRole(t *testing.T) {
	roles := map[string]RoleAccess{
		"author": {Read: true, Insert: true},
		"reader": {Read: true},
	}

	if !RoleCanDo(roles, "author", "read") {
		t.Fatal("author should be able to read")
	}
	if !RoleCanDo(roles, "author", "insert") {
		t.Fatal("author should be able to insert")
	}
	if RoleCanDo(roles, "author", "delete") {
		t.Fatal("author should not be able to delete")
	}

	if !RoleCanDo(roles, "reader", "read") {
		t.Fatal("reader should be able to read")
	}
	if RoleCanDo(roles, "reader", "insert") {
		t.Fatal("reader should not be able to insert")
	}
}

func TestRoleCanDoUnknownRole(t *testing.T) {
	roles := map[string]RoleAccess{
		"author": {Read: true, Insert: true},
	}
	if RoleCanDo(roles, "stranger", "read") {
		t.Fatal("unknown role should not have access")
	}
}

func TestRoleCanDoEmptyRolesMap(t *testing.T) {
	if RoleCanDo(nil, "editor", "read") {
		t.Fatal("nil roles map should deny access")
	}
	if RoleCanDo(map[string]RoleAccess{}, "editor", "read") {
		t.Fatal("empty roles map should deny access")
	}
	if !RoleCanDo(nil, "owner", "read") {
		t.Fatal("owner should still be allowed with nil roles map")
	}
}

func TestRoleCanDoUnknownOp(t *testing.T) {
	roles := map[string]RoleAccess{
		"author": {Read: true, Insert: true, Update: true, Delete: true},
	}
	if RoleCanDo(roles, "author", "drop") {
		t.Fatal("unknown operation should be denied")
	}
}

func TestRoleAccessRoundTrip(t *testing.T) {
	tbl := &Table{
		Name:      "posts",
		SystemKey: true,
		Columns:   []Column{{Name: "title", Type: "text", Required: true}},
		Access:    &Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
		Roles: map[string]RoleAccess{
			"contributor": {Read: true, Insert: true},
			"viewer":      {Read: true},
		},
	}

	data, err := tbl.JSON()
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseTable(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(parsed.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(parsed.Roles))
	}
	if !parsed.Roles["contributor"].Insert {
		t.Fatal("contributor should have insert")
	}
	if parsed.Roles["viewer"].Insert {
		t.Fatal("viewer should not have insert")
	}
}
