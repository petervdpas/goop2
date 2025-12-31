// internal/viewer/render/templates.go

package render

import (
	"embed"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	tmpl    *template.Template
	once    sync.Once
	initErr error
)

func InitTemplates() error {
	once.Do(func() {
		funcs := template.FuncMap{
			"shortID": func(id string) string {
				if len(id) <= 10 {
					return id
				}
				return id[:10] + "…"
			},
			"rfc3339":  func(t time.Time) string { return t.Format(time.RFC3339) },
			"isActive": func(active, key string) bool { return active == key },
			"trim":     strings.TrimSpace,

			// include renders a named template (e.g. "page.settings") and returns HTML.
			// This is required because Go templates cannot dynamically choose a template
			// name in the {{template ...}} action.
			"include": func(name string, data any) template.HTML {
				if tmpl == nil {
					return template.HTML(`<pre class="err">templates not initialized</pre>`)
				}
				var b strings.Builder
				if err := tmpl.ExecuteTemplate(&b, name, data); err != nil {
					return template.HTML(`<pre class="err">` + html.EscapeString(err.Error()) + `</pre>`)
				}
				return template.HTML(b.String())
			},
		}

		var err error
		tmpl, err = template.New("root").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
		if err != nil {
			initErr = err
			return
		}
	})
	return initErr
}

// Always execute the shared layout. Layout chooses the page body via .ContentTmpl.
func Render(w http.ResponseWriter, data any) {
	if err := InitTemplates(); err != nil {
		http.Error(w, fmt.Sprintf("template init error: %v", err), http.StatusInternalServerError)
		return
	}
	if tmpl == nil {
		http.Error(w, "templates not initialized", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
}
