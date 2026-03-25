package gql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

type PeerSource struct {
	PeerID string
	Tables []schema.Table
}

type PeerQueryFunc func(ctx context.Context, peerID string, table string, opts storage.SelectOpts) ([]map[string]any, error)

func (e *Engine) RebuildFederated(localTables []*schema.Table, peers []PeerSource, queryPeer PeerQueryFunc) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tableContributors := make(map[string][]PeerSource)
	tableSchemas := make(map[string][]schema.Table)

	for _, tbl := range localTables {
		tableContributors[tbl.Name] = append(tableContributors[tbl.Name], PeerSource{
			PeerID: e.selfID,
			Tables: []schema.Table{*tbl},
		})
		tableSchemas[tbl.Name] = append(tableSchemas[tbl.Name], *tbl)
	}

	for _, peer := range peers {
		for _, tbl := range peer.Tables {
			tableContributors[tbl.Name] = append(tableContributors[tbl.Name], PeerSource{
				PeerID: peer.PeerID,
				Tables: []schema.Table{tbl},
			})
			tableSchemas[tbl.Name] = append(tableSchemas[tbl.Name], tbl)
		}
	}

	queryFields := graphql.Fields{}
	mutationFields := graphql.Fields{}

	for name, schemas := range tableSchemas {
		contributors := tableContributors[name]
		peerIDs := make([]string, len(contributors))
		for i, c := range contributors {
			peerIDs[i] = c.PeerID
		}

		var merged *schema.Table
		if len(schemas) == 1 {
			s := schemas[0]
			merged = &s
		} else {
			merged = schema.MergeTable(name, schemas)
		}
		if merged == nil {
			continue
		}

		objType := tableToObject(merged)
		filterType := tableToFilter(merged)
		orderType := tableToOrderBy(merged)

		isLocalOnly := len(peerIDs) == 1 && peerIDs[0] == e.selfID
		if isLocalOnly {
			queryFields[name] = queryField(merged, objType, filterType, orderType, e.db)
			queryFields[name+"_by_pk"] = queryByPKField(merged, objType, e.db)
			mutationFields["insert_"+name] = insertField(merged, objType, e.db, e.selfID, e.selfEmail)
			mutationFields["update_"+name+"_by_pk"] = updateField(merged, objType, e.db)
			mutationFields["delete_"+name+"_by_pk"] = deleteField(merged, e.db)
		} else {
			queryFields[name] = federatedQueryField(merged, objType, filterType, orderType, peerIDs, e.selfID, e.db, queryPeer)
		}
	}

	if len(queryFields) == 0 {
		e.built = false
		return nil
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
		return fmt.Errorf("gql: build federated schema: %w", err)
	}
	e.schema = s
	e.built = true
	return nil
}

func federatedQueryField(tbl *schema.Table, objType *graphql.Object, filterType *graphql.InputObject, orderType *graphql.InputObject, peerIDs []string, selfID string, db *storage.DB, queryPeer PeerQueryFunc) *graphql.Field {
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

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			type peerResult struct {
				rows []map[string]any
				err  error
			}

			var wg sync.WaitGroup
			results := make([]peerResult, len(peerIDs))

			for i, pid := range peerIDs {
				wg.Add(1)
				go func(idx int, peerID string) {
					defer wg.Done()
					if peerID == selfID {
						rows, err := db.SelectPaged(opts)
						results[idx] = peerResult{rows: rows, err: err}
						return
					}
					if queryPeer == nil {
						return
					}
					rows, err := queryPeer(ctx, peerID, tbl.Name, opts)
					results[idx] = peerResult{rows: rows, err: err}
				}(i, pid)
			}
			wg.Wait()

			var merged []map[string]any
			for _, r := range results {
				if r.err != nil {
					continue
				}
				merged = append(merged, r.rows...)
			}

			if opts.Limit > 0 && len(merged) > opts.Limit {
				merged = merged[:opts.Limit]
			}

			return merged, nil
		},
	}
}

func DefaultPeerQueryFunc(baseURL string) PeerQueryFunc {
	client := &http.Client{Timeout: 5 * time.Second}

	return func(ctx context.Context, peerID string, table string, opts storage.SelectOpts) ([]map[string]any, error) {
		url := fmt.Sprintf("%s/api/p/%s/data/query", baseURL, peerID)

		body := map[string]any{
			"table": table,
		}
		if opts.Where != "" {
			body["where"] = opts.Where
			body["args"] = opts.Args
		}
		if opts.Limit > 0 {
			body["limit"] = opts.Limit
		}
		if opts.Offset > 0 {
			body["offset"] = opts.Offset
		}
		if len(opts.Columns) > 0 {
			body["columns"] = opts.Columns
		}

		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(jsonBody)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("peer %s unreachable: %w", peerID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("peer %s returned %d", peerID, resp.StatusCode)
		}

		var rows []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
			return nil, fmt.Errorf("peer %s: decode: %w", peerID, err)
		}
		return rows, nil
	}
}
