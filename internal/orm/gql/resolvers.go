package gql

import (
	"fmt"
	"strings"

	"github.com/graphql-go/graphql"
	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

func queryField(tbl *schema.Table, objType *graphql.Object, filterType *graphql.InputObject, orderType *graphql.InputObject, db *storage.DB) *graphql.Field {
	return &graphql.Field{
		Type: graphql.NewList(objType),
		Args: graphql.FieldConfigArgument{
			"where":    &graphql.ArgumentConfig{Type: filterType},
			"limit":    &graphql.ArgumentConfig{Type: graphql.Int},
			"offset":   &graphql.ArgumentConfig{Type: graphql.Int},
			"order_by": &graphql.ArgumentConfig{Type: orderType},
		},
		Resolve: func(p graphql.ResolveParams) (any, error) {
			opts := storage.SelectOpts{Table: tbl.Name}

			if limit, ok := p.Args["limit"].(int); ok {
				opts.Limit = limit
			}
			if offset, ok := p.Args["offset"].(int); ok {
				opts.Offset = offset
			}

			if where, ok := p.Args["where"].(map[string]any); ok && len(where) > 0 {
				clauses, args := buildWhere(where)
				opts.Where = strings.Join(clauses, " AND ")
				opts.Args = args
			}

			if orderBy, ok := p.Args["order_by"].(map[string]any); ok && len(orderBy) > 0 {
				opts.Order = buildOrderBy(orderBy)
			}

			return db.SelectPaged(opts)
		},
	}
}

func queryByPKField(tbl *schema.Table, objType *graphql.Object, db *storage.DB) *graphql.Field {
	return &graphql.Field{
		Type: objType,
		Args: graphql.FieldConfigArgument{
			"_id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
		Resolve: func(p graphql.ResolveParams) (any, error) {
			id, _ := p.Args["_id"].(int)
			rows, err := db.SelectPaged(storage.SelectOpts{
				Table: tbl.Name,
				Where: "_id = ?",
				Args:  []any{id},
				Limit: 1,
			})
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				return nil, nil
			}
			return rows[0], nil
		},
	}
}

func insertField(tbl *schema.Table, objType *graphql.Object, db *storage.DB, selfID string, selfEmail func() string) *graphql.Field {
	inputFields := graphql.InputObjectConfigFieldMap{}
	for _, col := range tbl.Columns {
		gt := schemaTypeToGraphQL(col.Type)
		if col.Auto {
			inputFields[col.Name] = &graphql.InputObjectFieldConfig{Type: gt}
		} else if col.Required || col.Key {
			inputFields[col.Name] = &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(gt)}
		} else {
			inputFields[col.Name] = &graphql.InputObjectFieldConfig{Type: gt}
		}
	}

	inputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   sanitizeName(tbl.Name) + "_insert_input",
		Fields: inputFields,
	})

	return &graphql.Field{
		Type: objType,
		Args: graphql.FieldConfigArgument{
			"object": &graphql.ArgumentConfig{Type: graphql.NewNonNull(inputType)},
		},
		Resolve: func(p graphql.ResolveParams) (any, error) {
			obj, ok := p.Args["object"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("object is required")
			}
			data := make(map[string]any, len(obj))
			for k, v := range obj {
				data[k] = v
			}
			email := ""
			if selfEmail != nil {
				email = selfEmail()
			}
			id, err := db.OrmInsert(tbl.Name, selfID, email, data)
			if err != nil {
				return nil, err
			}
			return db.OrmGet(tbl.Name, id)
		},
	}
}

func updateField(tbl *schema.Table, objType *graphql.Object, db *storage.DB) *graphql.Field {
	setFields := graphql.InputObjectConfigFieldMap{}
	for _, col := range tbl.Columns {
		if col.Key {
			continue
		}
		setFields[col.Name] = &graphql.InputObjectFieldConfig{Type: schemaTypeToGraphQL(col.Type)}
	}

	setType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   sanitizeName(tbl.Name) + "_set_input",
		Fields: setFields,
	})

	return &graphql.Field{
		Type: objType,
		Args: graphql.FieldConfigArgument{
			"_id":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
			"_set": &graphql.ArgumentConfig{Type: graphql.NewNonNull(setType)},
		},
		Resolve: func(p graphql.ResolveParams) (any, error) {
			id, _ := p.Args["_id"].(int)
			set, ok := p.Args["_set"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("_set is required")
			}
			data := make(map[string]any, len(set))
			for k, v := range set {
				data[k] = v
			}
			if err := db.OrmUpdate(tbl.Name, int64(id), data); err != nil {
				return nil, err
			}
			return db.OrmGet(tbl.Name, int64(id))
		},
	}
}

func deleteField(tbl *schema.Table, db *storage.DB) *graphql.Field {
	return &graphql.Field{
		Type: graphql.NewObject(graphql.ObjectConfig{
			Name: sanitizeName(tbl.Name) + "_delete_result",
			Fields: graphql.Fields{
				"affected_rows": &graphql.Field{Type: graphql.Int},
			},
		}),
		Args: graphql.FieldConfigArgument{
			"_id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
		Resolve: func(p graphql.ResolveParams) (any, error) {
			id, _ := p.Args["_id"].(int)
			if err := db.OrmDelete(tbl.Name, int64(id)); err != nil {
				return nil, err
			}
			return map[string]any{"affected_rows": 1}, nil
		},
	}
}

func buildWhere(where map[string]any) ([]string, []any) {
	var clauses []string
	var args []any

	for key, val := range where {
		parts := splitFilterKey(key)
		if parts.col == "" {
			continue
		}
		switch parts.op {
		case "eq":
			clauses = append(clauses, parts.col+" = ?")
			args = append(args, val)
		case "neq":
			clauses = append(clauses, parts.col+" != ?")
			args = append(args, val)
		case "gt":
			clauses = append(clauses, parts.col+" > ?")
			args = append(args, val)
		case "gte":
			clauses = append(clauses, parts.col+" >= ?")
			args = append(args, val)
		case "lt":
			clauses = append(clauses, parts.col+" < ?")
			args = append(args, val)
		case "lte":
			clauses = append(clauses, parts.col+" <= ?")
			args = append(args, val)
		case "like":
			clauses = append(clauses, parts.col+" LIKE ?")
			args = append(args, val)
		}
	}
	return clauses, args
}

type filterParts struct {
	col string
	op  string
}

func splitFilterKey(key string) filterParts {
	ops := []string{"_neq", "_gte", "_lte", "_gt", "_lt", "_eq", "_like"}
	for _, op := range ops {
		if strings.HasSuffix(key, op) {
			return filterParts{
				col: key[:len(key)-len(op)],
				op:  op[1:],
			}
		}
	}
	return filterParts{}
}

func buildOrderBy(orderBy map[string]any) string {
	var parts []string
	for col, dir := range orderBy {
		d, ok := dir.(string)
		if !ok {
			continue
		}
		parts = append(parts, col+" "+d)
	}
	return strings.Join(parts, ", ")
}
