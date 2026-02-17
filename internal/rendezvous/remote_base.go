package rendezvous

import (
	"net/http"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/util"
)

// remoteBase holds fields and methods common to all remote service providers.
// Embed this in a concrete provider and set fetchFn in the constructor.
type remoteBase struct {
	baseURL    string
	adminToken string
	client     *http.Client

	// status cache — populated by fetchFn
	version    string
	apiVersion int
	dummyMode  bool
	cachedAt   time.Time
	cacheMu    sync.RWMutex
	fetchFn    func() // provider-specific status fetcher
}

func newRemoteBase(baseURL, adminToken string) remoteBase {
	return remoteBase{
		baseURL:    util.NormalizeURL(baseURL),
		adminToken: adminToken,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

func (b *remoteBase) fetchStatus() {
	if b.fetchFn != nil {
		b.fetchFn()
	}
}

// Version returns the cached version string.
func (b *remoteBase) Version() string {
	b.fetchStatus()
	b.cacheMu.RLock()
	defer b.cacheMu.RUnlock()
	return b.version
}

// APIVersion returns the cached API version.
func (b *remoteBase) APIVersion() int {
	b.fetchStatus()
	b.cacheMu.RLock()
	defer b.cacheMu.RUnlock()
	return b.apiVersion
}

// DummyMode returns the cached dummy_mode flag.
func (b *remoteBase) DummyMode() bool {
	b.fetchStatus()
	b.cacheMu.RLock()
	defer b.cacheMu.RUnlock()
	return b.dummyMode
}
