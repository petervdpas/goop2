package rendezvous

import (
	"html/template"
	"net/http"
)

// StorePageData holds HTML fragments injected by the credit module into
// the template store page. The public repo controls layout; the private
// module controls content.
type StorePageData struct {
	// Banner is shown at the top of the store (e.g. balance, or "all free").
	Banner template.HTML

	// CreditPacks is the credit pack purchase section.
	// Empty when credits are disabled.
	CreditPacks template.HTML
}

// TemplateStoreInfo holds per-template credit information for the store page.
type TemplateStoreInfo struct {
	// PriceLabel is shown on each template card (e.g. "Free", "100 credits",
	// "Owned", or a buy button).
	PriceLabel template.HTML
}

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

	// StorePageData returns HTML fragments for the store page layout slots.
	// Called once per store page render.
	StorePageData(r *http.Request) StorePageData

	// TemplateStoreInfo returns per-template credit info for the store page.
	// Called once per template per store page render.
	TemplateStoreInfo(r *http.Request, tpl StoreMeta) TemplateStoreInfo
}

// NoCredits is the default CreditProvider: all templates are free,
// no credit endpoints are registered.
type NoCredits struct{}

func (NoCredits) RegisterRoutes(*http.ServeMux)                        {}
func (NoCredits) TemplateAccessAllowed(*http.Request, StoreMeta) bool  { return true }
func (NoCredits) StorePageData(*http.Request) StorePageData {
	return StorePageData{
		Banner: `<div class="store-banner store-banner-free">All templates on this server are free — no credits needed.</div>`,
	}
}
func (NoCredits) TemplateStoreInfo(_ *http.Request, _ StoreMeta) TemplateStoreInfo {
	return TemplateStoreInfo{
		PriceLabel: `<span class="tpl-price-free">Free</span>`,
	}
}
