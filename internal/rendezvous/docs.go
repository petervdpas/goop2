package rendezvous

import (
	"bytes"
	"embed"
	"html/template"
	"path"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

//go:embed all:docs
var docsFS embed.FS

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

// newDocSite reads all .md files from the embedded docs directory, renders
// them with goldmark, and returns a DocSite with pages sorted by filename
// prefix. All rendering happens once at startup.
func newDocSite() *DocSite {
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	entries, err := docsFS.ReadDir("docs")
	if err != nil {
		return &DocSite{BySlug: map[string]*DocPage{}}
	}

	site := &DocSite{BySlug: map[string]*DocPage{}}

	for i, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := docsFS.ReadFile(path.Join("docs", e.Name()))
		if err != nil {
			continue
		}

		// Slug: strip numeric prefix and extension.
		// "01-overview.md" -> "overview"
		name := strings.TrimSuffix(e.Name(), ".md")
		parts := strings.SplitN(name, "-", 2)
		slug := name
		if len(parts) == 2 {
			slug = parts[1]
		}

		// Title: first "# Heading" line.
		title := slug
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
			Slug:  slug,
			Title: title,
			Order: i,
			HTML:  template.HTML(buf.String()),
		}
		site.Pages = append(site.Pages, page)
		site.BySlug[slug] = &site.Pages[len(site.Pages)-1]
	}

	sort.Slice(site.Pages, func(i, j int) bool {
		return site.Pages[i].Order < site.Pages[j].Order
	})

	// Re-index after sort since slice addresses may have changed.
	for idx := range site.Pages {
		site.BySlug[site.Pages[idx].Slug] = &site.Pages[idx]
	}

	return site
}
