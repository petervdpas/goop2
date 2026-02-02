// internal/ui/viewmodels/editor.go

package viewmodels

type EditorVM struct {
	BaseVM
	CSRF string

	Path     string
	Content  string
	ETag     string
	SiteRoot string // absolute path to the site root directory

	Dir   string
	Files []EditorFileRow

	Tree  []EditorTreeRow
	Saved bool
	Error string
}

type EditorFileRow struct {
	Path  string // root-relative
	Size  int64
	ETag  string
	Mod   int64 // unix seconds
	IsDir bool
}

type EditorTreeRow struct {
	Path  string
	IsDir bool
	Depth int
}
