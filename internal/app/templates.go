package app

import (
	"log"

	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/util"
)

// setupTemplates configures the templates provider. When templatesURL is set,
// a remote templates service is used. Otherwise, if templatesDir is set,
// templates are loaded from disk and served directly (all free, no credits).
func setupTemplates(rv *rendezvous.Server, templatesURL, adminToken, templatesDir, peerDir string) {
	if templatesURL != "" {
		log.Printf("Templates service: %s", templatesURL)
		rv.SetTemplatesProvider(rendezvous.NewRemoteTemplatesProvider(templatesURL, adminToken))
		return
	}
	if templatesDir != "" {
		dir := util.ResolvePath(peerDir, templatesDir)
		store := rendezvous.NewLocalTemplateStore(dir)
		if store != nil {
			log.Printf("Local template store: %s (%d templates)", dir, store.Count())
			rv.SetLocalTemplateStore(store)
		}
	}
}
