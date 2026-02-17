
package render

import (
	"fmt"
	"html"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/ui"
)

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
		// IMPORTANT: ParseFS paths must match the embedded paths exactly.
		tmpl, err = template.New("root").Funcs(funcs).ParseFS(ui.TemplatesFS, "templates/*.html")
		if err != nil {
			initErr = err
			return
		}
	})
	return initErr
}

// RenderStandalone executes a named template directly (no layout wrapper).
// Use for standalone pages like error screens that don't need nav/footer.
func RenderStandalone(w http.ResponseWriter, name string, data any) {
	if err := InitTemplates(); err != nil {
		http.Error(w, fmt.Sprintf("template init error: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
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
