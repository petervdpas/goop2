package gql

import (
	"fmt"
	"strings"
	"sync"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"
	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

type Engine struct {
	db        *storage.DB
	selfID    string
	selfEmail func() string

	mu     sync.RWMutex
	schema graphql.Schema
	built  bool
}

func New(db *storage.DB, selfID string, selfEmail func() string) *Engine {
	return &Engine{
		db:        db,
		selfID:    selfID,
		selfEmail: selfEmail,
	}
}

func (e *Engine) Rebuild() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tables, err := e.contextTables()
	if err != nil {
		return fmt.Errorf("gql: list tables: %w", err)
	}

	if len(tables) == 0 {
		e.built = false
		return nil
	}

	queryFields := graphql.Fields{}
	mutationFields := graphql.Fields{}

	for _, tbl := range tables {
		objType := tableToObject(tbl)
		filterType := tableToFilter(tbl)
		orderType := tableToOrderBy(tbl)

		queryFields[tbl.Name] = queryField(tbl, objType, filterType, orderType, e.db)
		queryFields[tbl.Name+"_by_pk"] = queryByPKField(tbl, objType, e.db)

		mutationFields["insert_"+tbl.Name] = insertField(tbl, objType, e.db, e.selfID, e.selfEmail)
		mutationFields["update_"+tbl.Name+"_by_pk"] = updateField(tbl, objType, e.db)
		mutationFields["delete_"+tbl.Name+"_by_pk"] = deleteField(tbl, e.db)
	}

	cfg := graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: queryFields,
		}),
	}
	if len(mutationFields) > 0 {
		cfg.Mutation = graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: mutationFields,
		})
	}

	s, err := graphql.NewSchema(cfg)
	if err != nil {
		return fmt.Errorf("gql: build schema: %w", err)
	}
	e.schema = s
	e.built = true
	return nil
}

func (e *Engine) Execute(query string, variables map[string]any, operationName string) *graphql.Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.built {
		return &graphql.Result{
			Errors: []gqlerrors.FormattedError{
				{Message: "no tables in context — mark at least one schema with context: true"},
			},
		}
	}

	return graphql.Do(graphql.Params{
		Schema:         e.schema,
		RequestString:  query,
		VariableValues: variables,
		OperationName:  operationName,
	})
}

func (e *Engine) TableSDL(tableName string) string {
	tbl, err := e.db.GetSchema(tableName)
	if err != nil || tbl == nil || !tbl.Context {
		return ""
	}
	return generateSDL(tbl)
}

func TableSDLFromSchema(tbl *schema.Table) string {
	if tbl == nil {
		return ""
	}
	return generateSDL(tbl)
}

func generateSDL(tbl *schema.Table) string {
	name := sanitizeName(tbl.Name)
	var b strings.Builder

	b.WriteString("type " + name + " {\n")
	b.WriteString("  _id: Int\n")
	b.WriteString("  _owner: String\n")
	b.WriteString("  _owner_email: String\n")
	b.WriteString("  _created_at: String\n")
	b.WriteString("  _updated_at: String\n")
	for _, col := range tbl.Columns {
		gqlType := schemaTypeToSDL(col.Type)
		required := ""
		if col.Required || col.Key {
			required = "!"
		}
		b.WriteString("  " + col.Name + ": " + gqlType + required + "\n")
	}
	b.WriteString("}\n\n")

	b.WriteString("type Query {\n")
	b.WriteString("  " + name + "(where: " + name + "_where, limit: Int, offset: Int, order_by: " + name + "_order_by): [" + name + "!]!\n")
	b.WriteString("  " + name + "_by_pk(_id: Int!): " + name + "\n")
	b.WriteString("}\n\n")

	b.WriteString("type Mutation {\n")
	b.WriteString("  insert_" + name + "(object: " + name + "_insert_input!): " + name + "\n")
	b.WriteString("  update_" + name + "_by_pk(_id: Int!, _set: " + name + "_set_input!): " + name + "\n")
	b.WriteString("  delete_" + name + "_by_pk(_id: Int!): " + name + "_delete_result\n")
	b.WriteString("}\n")

	return b.String()
}

func schemaTypeToSDL(t string) string {
	switch strings.ToLower(t) {
	case "integer":
		return "Int"
	case "real":
		return "Float"
	case "text", "guid", "datetime", "date", "time", "enum":
		return "String"
	case "blob":
		return "String"
	default:
		return "String"
	}
}

func (e *Engine) ContextTables() []*schema.Table {
	tables, _ := e.contextTables()
	return tables
}

func (e *Engine) ContextTableNames() []string {
	tables, err := e.contextTables()
	if err != nil {
		return nil
	}
	names := make([]string, len(tables))
	for i, t := range tables {
		names[i] = t.Name
	}
	return names
}

func (e *Engine) contextTables() ([]*schema.Table, error) {
	tables, err := e.db.ListTables()
	if err != nil {
		return nil, err
	}

	var result []*schema.Table
	for _, t := range tables {
		tbl, err := e.db.GetSchema(t.Name)
		if err != nil || tbl == nil {
			continue
		}
		if tbl.Context {
			result = append(result, tbl)
		}
	}
	return result, nil
}

func tableToObject(tbl *schema.Table) *graphql.Object {
	fields := graphql.Fields{
		"_id":          &graphql.Field{Type: graphql.Int},
		"_owner":       &graphql.Field{Type: graphql.String},
		"_owner_email": &graphql.Field{Type: graphql.String},
		"_created_at":  &graphql.Field{Type: graphql.String},
		"_updated_at":  &graphql.Field{Type: graphql.String},
	}
	for _, col := range tbl.Columns {
		fields[col.Name] = &graphql.Field{Type: schemaTypeToGraphQL(col.Type)}
	}
	return graphql.NewObject(graphql.ObjectConfig{
		Name:   sanitizeName(tbl.Name),
		Fields: fields,
	})
}

func tableToFilter(tbl *schema.Table) *graphql.InputObject {
	fields := graphql.InputObjectConfigFieldMap{}

	allCols := append([]schema.Column{
		{Name: "_id", Type: "integer"},
		{Name: "_owner", Type: "text"},
		{Name: "_owner_email", Type: "text"},
		{Name: "_created_at", Type: "datetime"},
		{Name: "_updated_at", Type: "datetime"},
	}, tbl.Columns...)

	for _, col := range allCols {
		gt := schemaTypeToGraphQL(col.Type)
		fields[col.Name+"_eq"] = &graphql.InputObjectFieldConfig{Type: gt}
		fields[col.Name+"_neq"] = &graphql.InputObjectFieldConfig{Type: gt}

		switch col.Type {
		case "integer", "real", "datetime", "date", "time":
			fields[col.Name+"_gt"] = &graphql.InputObjectFieldConfig{Type: gt}
			fields[col.Name+"_gte"] = &graphql.InputObjectFieldConfig{Type: gt}
			fields[col.Name+"_lt"] = &graphql.InputObjectFieldConfig{Type: gt}
			fields[col.Name+"_lte"] = &graphql.InputObjectFieldConfig{Type: gt}
		case "text", "guid", "enum":
			fields[col.Name+"_like"] = &graphql.InputObjectFieldConfig{Type: graphql.String}
		}
	}

	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   sanitizeName(tbl.Name) + "_where",
		Fields: fields,
	})
}

var orderEnum = graphql.NewEnum(graphql.EnumConfig{
	Name: "OrderDirection",
	Values: graphql.EnumValueConfigMap{
		"asc":  &graphql.EnumValueConfig{Value: "ASC"},
		"desc": &graphql.EnumValueConfig{Value: "DESC"},
	},
})

func tableToOrderBy(tbl *schema.Table) *graphql.InputObject {
	fields := graphql.InputObjectConfigFieldMap{}
	allNames := []string{"_id", "_created_at", "_updated_at"}
	for _, col := range tbl.Columns {
		allNames = append(allNames, col.Name)
	}
	for _, name := range allNames {
		fields[name] = &graphql.InputObjectFieldConfig{Type: orderEnum}
	}
	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   sanitizeName(tbl.Name) + "_order_by",
		Fields: fields,
	})
}

func schemaTypeToGraphQL(t string) graphql.Output {
	switch strings.ToLower(t) {
	case "integer":
		return graphql.Int
	case "real":
		return graphql.Float
	case "text", "guid", "datetime", "date", "time", "enum":
		return graphql.String
	case "blob":
		return graphql.String
	default:
		return graphql.String
	}
}

func sanitizeName(name string) string {
	var b strings.Builder
	for i, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' {
			b.WriteRune(r)
		} else if r >= '0' && r <= '9' && i > 0 {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
