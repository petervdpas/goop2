package app

import "github.com/petervdpas/goop2/internal/rendezvous"

// setupCredits is a no-op in the public repo. The private goop2-credits
// module replaces this file to wire in the SQLite-backed credit provider.
func setupCredits(_ *rendezvous.Server, _ string) {}
