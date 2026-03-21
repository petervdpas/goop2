package docs

import (
	"bytes"
	"embed"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

//go:embed sdk.md lua.md
var docsFS embed.FS

// Rendered holds pre-rendered HTML for the viewer docs tabs.
type Rendered struct {
	SDK template.HTML
	Lua template.HTML
}

// Render reads the embedded markdown files and returns pre-rendered HTML
// with syntax-highlighted code blocks. Called once at startup.
func Render() *Rendered {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			highlighting.NewHighlighting(
				highlighting.WithStyle("dracula"),
			),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	return &Rendered{
		SDK: renderFile(md, "sdk.md"),
		Lua: renderFile(md, "lua.md"),
	}
}

func renderFile(md goldmark.Markdown, name string) template.HTML {
	data, err := docsFS.ReadFile(name)
	if err != nil {
		return template.HTML("<p>Failed to load " + name + "</p>")
	}
	var buf bytes.Buffer
	if err := md.Convert(data, &buf); err != nil {
		return template.HTML("<p>Failed to render " + name + "</p>")
	}
	return template.HTML(buf.String())
}
