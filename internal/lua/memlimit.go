package lua

import (
	"context"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// memoryMonitor watches process memory growth during Lua script execution
// and kills the VM if the allocation delta exceeds the configured limit.
//
// Because gopher-lua has no per-VM memory tracking, this uses Go's
// runtime.MemStats as an approximation. The measurement is process-wide,
// so concurrent scripts could influence each other's readings. This is
// acceptable as a safety net against runaway allocations.
type memoryMonitor struct {
	limitBytes uint64
	baseline   uint64
	exceeded   atomic.Bool
}

// newMemoryMonitor creates a monitor that will trigger when process memory
// grows by more than maxMB megabytes from the current baseline. Returns nil
// if maxMB <= 0 (monitoring disabled).
func newMemoryMonitor(maxMB int) *memoryMonitor {
	if maxMB <= 0 {
		return nil
	}

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	return &memoryMonitor{
		limitBytes: uint64(maxMB) * 1024 * 1024,
		baseline:   stats.Alloc,
	}
}

// watch periodically checks memory and closes the Lua VM if the limit is
// exceeded. Returns a cancel function to stop the monitor.
func (m *memoryMonitor) watch(ctx context.Context, L *glua.LState, scriptName string) context.CancelFunc {
	if m == nil {
		return func() {}
	}

	monCtx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-monCtx.Done():
				return
			case <-ticker.C:
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)

				delta := uint64(0)
				if stats.Alloc > m.baseline {
					delta = stats.Alloc - m.baseline
				}

				if delta > m.limitBytes {
					m.exceeded.Store(true)
					log.Printf("LUA: memory limit exceeded for %s (delta=%dMB, limit=%dMB), killing VM",
						scriptName, delta/(1024*1024), m.limitBytes/(1024*1024))
					L.Close()
					return
				}
			}
		}
	}()

	return cancel
}

// wasExceeded returns true if the memory limit was breached.
func (m *memoryMonitor) wasExceeded() bool {
	if m == nil {
		return false
	}
	return m.exceeded.Load()
}
