package lua

import (
	"sync"
	"time"
)

// rateLimiter implements a sliding window rate limiter for per-peer and global limits.
type rateLimiter struct {
	mu       sync.Mutex
	perPeer  map[string][]time.Time
	global   []time.Time
	peerMax  int
	globalMax int
	window   time.Duration
}

func newRateLimiter(perPeerPerMin, globalPerMin int) *rateLimiter {
	return &rateLimiter{
		perPeer:   make(map[string][]time.Time),
		peerMax:   perPeerPerMin,
		globalMax: globalPerMin,
		window:    time.Minute,
	}
}

// AllowFunc checks rate limits with per-function granularity.
// funcLimit: -1 = use default peerMax, 0 = no limiting, N>0 = custom limit.
func (r *rateLimiter) AllowFunc(peerID, function string, funcLimit int) bool {
	if funcLimit == 0 {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Enforce global limit
	r.global = pruneOld(r.global, cutoff)
	if len(r.global) >= r.globalMax {
		return false
	}

	// Per-function per-peer limit
	key := peerID + ":" + function
	limit := r.peerMax
	if funcLimit > 0 {
		limit = funcLimit
	}

	r.perPeer[key] = pruneOld(r.perPeer[key], cutoff)
	if len(r.perPeer[key]) >= limit {
		return false
	}

	// Record
	r.global = append(r.global, now)
	r.perPeer[key] = append(r.perPeer[key], now)
	return true
}

func pruneOld(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	if i == 0 {
		return ts
	}
	return append(ts[:0:0], ts[i:]...)
}
