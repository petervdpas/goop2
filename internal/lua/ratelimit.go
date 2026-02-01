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

// Allow returns true if the request from peerID is within both per-peer and global limits.
func (r *rateLimiter) Allow(peerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Prune and check global
	r.global = pruneOld(r.global, cutoff)
	if len(r.global) >= r.globalMax {
		return false
	}

	// Prune and check per-peer
	r.perPeer[peerID] = pruneOld(r.perPeer[peerID], cutoff)
	if len(r.perPeer[peerID]) >= r.peerMax {
		return false
	}

	// Record
	r.global = append(r.global, now)
	r.perPeer[peerID] = append(r.perPeer[peerID], now)
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
