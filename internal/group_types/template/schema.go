package template

import (
	"encoding/json"
	"strings"

	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
)

// SchemaInfo holds the results of analyzing a template's schema files.
type SchemaInfo struct {
	NeedsGroup bool
	Roles      []string
}

// AnalyzeSchemas inspects schema files and legacy table policies to determine
// whether a group is needed and which roles the schemas define.
func AnalyzeSchemas(schemaFiles map[string][]byte, tablePolicies map[string]string) SchemaInfo {
	roleSet := map[string]bool{}
	needsGroup := false

	for rel, data := range schemaFiles {
		if !strings.HasPrefix(rel, "schemas/") || !strings.HasSuffix(rel, ".json") {
			continue
		}
		var tbl ormschema.Table
		if json.Unmarshal(data, &tbl) != nil {
			continue
		}
		if tbl.Access != nil && tbl.Access.UsesGroup() {
			needsGroup = true
		}
		for roleName := range tbl.Roles {
			needsGroup = true
			roleSet[roleName] = true
		}
	}

	for _, policy := range tablePolicies {
		if policy == "group" {
			needsGroup = true
			break
		}
	}

	roles := make([]string, 0, len(roleSet))
	for r := range roleSet {
		roles = append(roles, r)
	}

	return SchemaInfo{NeedsGroup: needsGroup, Roles: roles}
}
