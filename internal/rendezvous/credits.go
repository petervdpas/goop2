package rendezvous

import "github.com/petervdpas/goop2/creditapi"

// Type aliases so that existing code within internal/ keeps compiling
// without import changes, while the canonical definitions live in the
// public creditapi package.
type (
	StoreMeta         = creditapi.StoreMeta
	TablePolicy       = creditapi.TablePolicy
	StorePageData     = creditapi.StorePageData
	TemplateStoreInfo = creditapi.TemplateStoreInfo
	CreditProvider    = creditapi.CreditProvider
	NoCredits         = creditapi.NoCredits
)
