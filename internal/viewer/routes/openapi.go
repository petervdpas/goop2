package routes

import (
	_ "embed"
	"net/http"

	// Import the generated docs so swag's init() registers the spec.
	_ "github.com/petervdpas/goop2/docs"

	"github.com/swaggo/swag"
)

//go:embed executor_api.yaml
var executorAPISpec []byte

//go:embed executor_examples.html
var executorExamplesHTML []byte

// RegisterOpenAPI serves the Swagger 2.0 spec at GET /api/openapi.json
// and the standalone Executor API spec at GET /api/executor-api.yaml.
func RegisterOpenAPI(mux *http.ServeMux) {
	handleGet(mux, "/api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		doc, err := swag.ReadDoc()
		if err != nil {
			http.Error(w, "spec unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte(doc))
	})

	handleGet(mux, "/api/executor-api.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(executorAPISpec)
	})

	handleGet(mux, "/api/executor-examples", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(executorExamplesHTML)
	})
}
