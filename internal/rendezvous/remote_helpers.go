package rendezvous

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// readJSON reads resp.Body and unmarshals JSON into v.
func readJSON(resp *http.Response, v any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// setAuthHeader sets Bearer authorization if token is non-empty.
func setAuthHeader(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// fetchCachedStatus fetches a JSON status endpoint with 30s TTL caching.
// result must be a pointer for json.Unmarshal.
// apply is called under the write lock to copy decoded fields into the provider.
func fetchCachedStatus(mu *sync.RWMutex, cachedAt *time.Time,
	client *http.Client, url, logPrefix string, result any, apply func()) {

	const cacheTTL = 30 * time.Second

	mu.RLock()
	if time.Since(*cachedAt) < cacheTTL {
		mu.RUnlock()
		return
	}
	mu.RUnlock()

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("%s: status check error: %v", logPrefix, err)
		return
	}
	defer resp.Body.Close()

	if err := readJSON(resp, result); err != nil {
		return
	}

	mu.Lock()
	apply()
	*cachedAt = time.Now()
	mu.Unlock()
}
