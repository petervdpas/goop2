package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/orm/gql"
	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/ui/render"
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

	handlePost(mux, "/api/graphql/schema", func(w http.ResponseWriter, r *http.Request, req schema.Table) {
		if req.Name == "" || len(req.Columns) == 0 {
			http.Error(w, "name and columns required", http.StatusBadRequest)
			return
		}
		sdl := gql.TableSDLFromSchema(&req)
		writeJSON(w, map[string]string{
			"sdl":  sdl,
			"html": render.Highlight(sdl, "graphql"),
		})
	})

	handleGet(mux, "/api/graphql/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"enabled": true,
			"tables":  engine.ContextTableNames(),
		})
	})
}
