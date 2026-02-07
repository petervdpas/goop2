// internal/ui/viewmodels/templates.go

package viewmodels

import (
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/sitetemplates"
)

type TemplatesVM struct {
	BaseVM
	CSRF           string
	Templates      []sitetemplates.TemplateMeta
	StoreTemplates []rendezvous.StoreMeta
	StoreError     string
}
