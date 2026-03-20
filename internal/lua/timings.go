package lua

import "time"

// Lua scripting engine timings.
const (
	HTTPTimeout      = 5 * time.Second        // Lua http.get/post calls
	ShutdownTimeout  = 500 * time.Millisecond // graceful VM shutdown wait
	MemCheckInterval = 100 * time.Millisecond // memory limit enforcement poll
	RateLimitWindow  = time.Minute            // sliding window for rate limiting
)
