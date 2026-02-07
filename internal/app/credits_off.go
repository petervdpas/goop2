//go:build !credits

package app

import "github.com/petervdpas/goop2/internal/rendezvous"

func setupCredits(_ *rendezvous.Server, _ string) {
	// Credits module not included in this build.
	// Build with -tags credits to enable.
}
