package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/orm/gql"
)

func RegisterGraphQL(mux *http.ServeMux, engine *gql.Engine) {
	if engine == nil {
		return
	}

	handlePost(mux, "/api/graphql", func(w http.ResponseWriter, r *http.Request, req struct {
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
		OperationName string         `json:"operationName"`
	}) {
		result := engine.Execute(req.Query, req.Variables, req.OperationName)
		writeJSON(w, result)
	})

	handlePostAction(mux, "/api/graphql/rebuild", func(w http.ResponseWriter, r *http.Request) {
		if err := engine.Rebuild(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "rebuilt"})
	})

	handleGet(mux, "/api/graphql/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"enabled": true,
			"tables":  engine.ContextTableNames(),
		})
	})
}
