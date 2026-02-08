package app

import (
	"log"

	"github.com/petervdpas/goop2/internal/rendezvous"
)

// setupEmail configures the email provider. When emailURL is set,
// a remote email service is used for sending emails.
func setupEmail(rv *rendezvous.Server, emailURL string) {
	if emailURL == "" {
		return
	}
	log.Printf("Email service: %s", emailURL)
	rv.SetEmailProvider(rendezvous.NewRemoteEmailProvider(emailURL))
}
