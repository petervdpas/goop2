package app

import "log"

// setupMicroService configures an external micro-service provider.
// If url is empty the service is skipped. Otherwise it logs the
// service name and calls configure to wire the provider.
func setupMicroService(name, url string, configure func()) {
	if url == "" {
		return
	}
	log.Printf("%s service: %s", name, url)
	configure()
}
