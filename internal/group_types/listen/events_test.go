package listen

import (
	"testing"
)

func TestFlags(t *testing.T) {
	m := &Manager{}
	if !m.Flags().HostCanJoin {
		t.Fatal("listen handler should allow host to join")
	}
}
