package app

import (
	"log"

	"github.com/petervdpas/goop2/internal/rendezvous"
)

// setupRegistration configures the registration provider. When registrationURL
// is set, a remote registration service is used. Otherwise registration is disabled.
func setupRegistration(rv *rendezvous.Server, registrationURL, adminToken string) {
	if registrationURL == "" {
		return
	}
	log.Printf("Registration service: %s", registrationURL)
	rv.SetRegistrationProvider(rendezvous.NewRemoteRegistrationProvider(registrationURL, adminToken))
}
