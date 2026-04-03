package datafed

import (
	"testing"
)

func TestSchemaWithdraw(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "Fed", 0, false)

	fg := m.groups["g1"]
	fg.rwmu.Lock()
	fg.contributions["peer-a"] = &PeerContribution{PeerID: "peer-a"}
	fg.rwmu.Unlock()

	m.handleSchemaWithdraw("g1", "peer-a")

	contribs := m.GroupContributions("g1")
	if len(contribs) != 0 {
		t.Fatalf("expected 0 contributions after withdraw, got %d", len(contribs))
	}
}
