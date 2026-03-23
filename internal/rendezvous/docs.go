package rendezvous

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/petervdpas/goop2/internal/shareddocs"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

// pageOrder defines the display order and slug for each doc page.
var pageOrder = []struct {
	File string
	Slug string
}{
	{"overview.md", "overview"},
	{"quickstart.md", "quickstart"},
	{"connecting.md", "connecting"},
	{"configuration.md", "configuration"},
	{"templates.md", "templates"},
	{"lua.md", "scripting"},
	{"groups.md", "groups"},
	{"advanced.md", "advanced"},
	{"faq.md", "faq"},
	{"api.md", "api"},
	{"executor.md", "executor"},
	{"sdk.md", "sdk"},
}

// DocPage holds a single rendered documentation page.
type DocPage struct {
	Slug  string
	Title string
	Order int
	HTML  template.HTML
}

// DocSite holds all documentation pages, rendered at startup.
type DocSite struct {
	Pages  []DocPage
	BySlug map[string]*DocPage
}

// newDocSite reads all doc pages from the centralized docs package,
// renders them with goldmark, and returns a DocSite ordered by
// the defined page sequence. All rendering happens once at startup.
func newDocSite() *DocSite {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			highlighting.NewHighlighting(
				highlighting.WithStyle("dracula"),
			),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	site := &DocSite{BySlug: map[string]*DocPage{}}

	for i, entry := range pageOrder {
		data, err := shareddocs.Shared.ReadFile(entry.File)
		if err != nil {
			continue
		}

		title := entry.Slug
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimPrefix(line, "# ")
				break
			}
		}

		var buf bytes.Buffer
		if err := md.Convert(data, &buf); err != nil {
			continue
		}

		page := DocPage{
			Slug:  entry.Slug,
			Title: title,
			Order: i,
			HTML:  template.HTML(buf.String()),
		}
		site.Pages = append(site.Pages, page)
		site.BySlug[entry.Slug] = &site.Pages[len(site.Pages)-1]
	}

	return site
}
