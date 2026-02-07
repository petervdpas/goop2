// internal/viewer/routes/data_proxy.go
package routes

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/storage"
)

// RegisterDataProxy mounts the /api/p/ prefix that relays data API calls
// to a remote peer via the P2P data protocol.
func RegisterDataProxy(mux *http.ServeMux, node *p2p.Node) {
	mux.HandleFunc("/api/p/", func(w http.ResponseWriter, r *http.Request) {
		// Parse: /api/p/<peerID>/data/<operation...>
		path := strings.TrimPrefix(r.URL.Path, "/api/p/")
		slash := strings.IndexByte(path, '/')
		if slash < 0 {
			http.Error(w, "missing peer id", http.StatusBadRequest)
			return
		}
		peerID := path[:slash]
		rest := path[slash:] // e.g. /data/insert

		if !strings.HasPrefix(rest, "/data/") && rest != "/data" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		suffix := strings.TrimPrefix(rest, "/data")
		suffix = strings.TrimPrefix(suffix, "/")

		op := mapSuffixToOp(suffix)
		if op == "" {
			http.Error(w, "unknown data operation", http.StatusNotFound)
			return
		}

		req, err := buildDataRequest(op, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var resp p2p.DataResponse
		if peerID == node.ID() {
			// Local shortcut: operate on own database directly
			resp = node.LocalDataOp(node.ID(), req)
		} else {
			resp, err = node.RemoteDataOp(r.Context(), peerID, req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}

		if !resp.OK {
			http.Error(w, resp.Error, http.StatusInternalServerError)
			return
		}

		writeJSON(w, resp.Data)
	})
}

func mapSuffixToOp(suffix string) string {
	switch suffix {
	case "tables":
		return "tables"
	case "insert":
		return "insert"
	case "query":
		return "query"
	case "update":
		return "update"
	case "delete":
		return "delete"
	case "tables/create":
		return "create-table"
	case "tables/describe":
		return "describe"
	case "tables/delete":
		return "delete-table"
	case "tables/add-column":
		return "add-column"
	case "tables/drop-column":
		return "drop-column"
	case "tables/rename":
		return "rename-table"
	case "lua/call":
		return "lua-call"
	case "lua/list":
		return "lua-list"
	}
	return ""
}

func buildDataRequest(op string, r *http.Request) (p2p.DataRequest, error) {
	req := p2p.DataRequest{Op: op}

	// GET ops with no body
	if op == "tables" || op == "lua-list" {
		return req, nil
	}

	// All other ops expect a JSON body
	var body struct {
		Table    string             `json:"table"`
		Name     string             `json:"name"`
		Data     map[string]any     `json:"data"`
		ID       int64              `json:"id"`
		Where    string             `json:"where"`
		Args     []any              `json:"args"`
		Columns  any                `json:"columns"`
		Column   *storage.ColumnDef `json:"column"`
		Limit    int                `json:"limit"`
		Offset   int                `json:"offset"`
		OldName  string             `json:"old_name"`
		NewName  string             `json:"new_name"`
		Function string             `json:"function"`
		Params   map[string]any     `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return req, err
	}

	req.Table = body.Table
	req.Name = body.Name
	req.Data = body.Data
	req.ID = body.ID
	req.Where = body.Where
	req.Args = body.Args
	req.Column = body.Column
	req.Limit = body.Limit
	req.Offset = body.Offset
	req.OldName = body.OldName
	req.NewName = body.NewName
	req.Function = body.Function
	req.Params = body.Params

	// "columns" can be either []string (for query) or []ColumnDef (for create-table)
	if body.Columns != nil {
		switch v := body.Columns.(type) {
		case []any:
			if op == "create-table" {
				// Re-marshal and unmarshal as []ColumnDef
				b, _ := json.Marshal(v)
				var defs []storage.ColumnDef
				if err := json.Unmarshal(b, &defs); err == nil {
					req.ColumnDefs = defs
				}
			} else {
				// Treat as []string for query
				for _, item := range v {
					if s, ok := item.(string); ok {
						req.Columns = append(req.Columns, s)
					}
				}
			}
		}
	}

	return req, nil
}
