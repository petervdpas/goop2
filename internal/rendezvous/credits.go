package rendezvous

import (
	"html/template"
	"net/http"
)

// StoreMeta holds metadata for a store template.
type StoreMeta struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Icon        string                 `json:"icon"`
	Dir         string                 `json:"dir"`
	Source      string                 `json:"source"`
	Tables      map[string]TablePolicy `json:"tables,omitempty"`
}

// TablePolicy holds per-table configuration from a template manifest.
type TablePolicy struct {
	InsertPolicy string `json:"insert_policy"`
}

// StorePageData holds HTML fragments injected by the credit module into
// the template store page.
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
type CreditProvider interface {
	// RegisterRoutes mounts credit-related HTTP endpoints on the server mux.
	RegisterRoutes(mux *http.ServeMux)

	// TemplateAccessAllowed is called before serving a template bundle.
	// Return true to allow download, false to block (server returns 402).
	TemplateAccessAllowed(r *http.Request, tpl StoreMeta) bool

	// StorePageData returns HTML fragments for the store page layout slots.
	StorePageData(r *http.Request) StorePageData

	// TemplateStoreInfo returns per-template credit info for the store page.
	TemplateStoreInfo(r *http.Request, tpl StoreMeta) TemplateStoreInfo
}

// NoCredits is the default CreditProvider: all templates are free,
// no credit endpoints are registered.
type NoCredits struct{}

func (NoCredits) RegisterRoutes(*http.ServeMux)                       {}
func (NoCredits) TemplateAccessAllowed(*http.Request, StoreMeta) bool { return true }
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
