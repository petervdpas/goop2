
package viewmodels

import (
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/sitetemplates"
)

type TemplatesVM struct {
	BaseVM
	CSRF                 string
	Templates            []sitetemplates.TemplateMeta
	StoreTemplates       []rendezvous.StoreMeta
	StoreTemplatePrices  map[string]int // dir → credits (from credits service)
	HasCredits           bool           // true when credit system is active
	StoreError string
}
