package app

import (
	"log"

	"github.com/petervdpas/goop2/internal/rendezvous"
)

// setupCredits configures the credit provider. When creditsURL is set,
// a remote credit service is used; otherwise credits are disabled (all free).
func setupCredits(rv *rendezvous.Server, creditsURL string) {
	if creditsURL == "" {
		return
	}
	log.Printf("Credits service: %s", creditsURL)
	rv.SetCreditProvider(rendezvous.NewRemoteCreditProvider(creditsURL))
}
