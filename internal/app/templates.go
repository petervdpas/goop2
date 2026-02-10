package app

import (
	"log"

	"github.com/petervdpas/goop2/internal/rendezvous"
)

// setupTemplates configures the templates provider. When templatesURL is set,
// a remote templates service is used for template management.
func setupTemplates(rv *rendezvous.Server, templatesURL, adminToken string) {
	if templatesURL == "" {
		return
	}
	log.Printf("Templates service: %s", templatesURL)
	rv.SetTemplatesProvider(rendezvous.NewRemoteTemplatesProvider(templatesURL, adminToken))
}
