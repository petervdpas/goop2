//go:build credits

package app

import (
	"log"

	credits "github.com/petervdpas/goop2-credits"

	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/util"
)

func setupCredits(rv *rendezvous.Server, peerDir string) {
	creditDB := util.ResolvePath(peerDir, "data/credits.db")
	cp, err := credits.New(creditDB)
	if err != nil {
		log.Printf("WARNING: credits module: %v (running without credits)", err)
		return
	}
	rv.SetCreditProvider(cp)
}
