// internal/ui/viewmodels/templates.go

package viewmodels

import (
	"goop/internal/rendezvous"
	"goop/internal/sitetemplates"
)

type TemplatesVM struct {
	BaseVM
	CSRF           string
	Templates      []sitetemplates.TemplateMeta
	StoreTemplates []rendezvous.StoreMeta
	StoreError     string
}
