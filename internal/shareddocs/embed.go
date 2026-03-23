// Package shareddocs provides all documentation as embedded markdown.
// Both the viewer (Create > Docs) and the rendezvous server
// (public docs site) render from this single source.
package shareddocs

import "embed"

//go:embed *.md
var Shared embed.FS
