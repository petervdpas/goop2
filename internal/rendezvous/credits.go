package rendezvous

import "net/http"

// CreditProvider is the interface that a credit/monetization system must
// implement. The default (NoCredits) allows all access — every template is
// free and no credit routes are registered.
//
// A private module can implement this interface and plug it in via
// Server.SetCreditProvider before calling Start.
type CreditProvider interface {
	// RegisterRoutes mounts credit-related HTTP endpoints on the server mux
	// (e.g. /api/credits/balance, /api/credits/purchase, etc.).
	RegisterRoutes(mux *http.ServeMux)

	// TemplateAccessAllowed is called before serving a template bundle.
	// The implementation can extract peer identity from the request
	// (headers, query params, etc.) and check the template metadata.
	// Return true to allow download, false to block (server returns 402).
	TemplateAccessAllowed(r *http.Request, tpl StoreMeta) bool
}

// NoCredits is the default CreditProvider: all templates are free,
// no credit endpoints are registered.
type NoCredits struct{}

func (NoCredits) RegisterRoutes(*http.ServeMux)                        {}
func (NoCredits) TemplateAccessAllowed(*http.Request, StoreMeta) bool { return true }
