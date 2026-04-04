package groups

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/storage"
)

// world holds test state shared across steps.
type world struct {
	db  *storage.DB
	mgr *group.Manager
	dir string
}

var w *world

func aFreshDatabase() error {
	dir, err := os.MkdirTemp("", "godog-groups-*")
	if err != nil {
		return err
	}
	db, err := storage.Open(dir)
	if err != nil {
		return err
	}
	mgr := group.NewTestManager(db, "self-peer-id")
	w = &world{db: db, mgr: mgr, dir: dir}
	return nil
}

func cleanup() {
	if w != nil {
		w.mgr.Close()
		w.db.Close()
		os.RemoveAll(w.dir)
		w = nil
	}
}

// ── Given steps ────────────────────────────────────────────────────────────

func aRegisteredVolatileType(typeName string) error {
	w.mgr.RegisterType(typeName, &volatileHandler{})
	return nil
}

func iHaveASubscription(groupID, name, groupType, host string) error {
	return w.db.AddSubscription(host, groupID, name, groupType, 0, false, "member", host)
}

func iHaveAnActiveConnection(groupID, host, groupType string) error {
	w.mgr.SetActiveConn(groupID, host, groupType)
	return nil
}

func iHostAGroup(groupID, name, groupType string) error {
	return w.mgr.CreateGroup(groupID, name, groupType, "", 0)
}

func iHostAGroupWithMax(groupID, name, groupType string, max int) error {
	return w.mgr.CreateGroup(groupID, name, groupType, "", max)
}

func iHaveJoinedMyOwnGroup(groupID string) error {
	return w.mgr.JoinOwnGroup(groupID)
}

func remotePeerJoinsGroup(peerID, groupID string) error {
	w.mgr.SimulateJoin(peerID, groupID)
	return nil
}

// ── When steps ─────────────────────────────────────────────────────────────

func iReceiveAnInvite(groupID, name, groupType, host string) error {
	w.mgr.SimulateInvite(host, map[string]any{
		"group_id":      groupID,
		"group_name":    name,
		"host_peer_id":  host,
		"group_type":    groupType,
	})
	return nil
}

func theHostClosesGroup(groupID string) error {
	w.mgr.SimulateHostClose(groupID)
	return nil
}

func remotePeerLeavesGroup(peerID, groupID string) error {
	w.mgr.SimulateLeave(peerID, groupID)
	return nil
}

// ── Then steps ─────────────────────────────────────────────────────────────

func iShouldHaveNSubscriptions(n int) error {
	subs, err := w.db.ListSubscriptions()
	if err != nil {
		return err
	}
	if len(subs) != n {
		return fmt.Errorf("expected %d subscriptions, got %d", n, len(subs))
	}
	return nil
}

func subscriptionShouldHaveName(groupID, expected string) error {
	subs, _ := w.db.ListSubscriptions()
	for _, s := range subs {
		if s.GroupID == groupID {
			if s.GroupName != expected {
				return fmt.Errorf("subscription %s: expected name %q, got %q", groupID, expected, s.GroupName)
			}
			return nil
		}
	}
	return fmt.Errorf("subscription %s not found", groupID)
}

func subscriptionShouldHaveHost(groupID, expected string) error {
	subs, _ := w.db.ListSubscriptions()
	for _, s := range subs {
		if s.GroupID == groupID {
			if s.HostPeerID != expected {
				return fmt.Errorf("subscription %s: expected host %q, got %q", groupID, expected, s.HostPeerID)
			}
			return nil
		}
	}
	return fmt.Errorf("subscription %s not found", groupID)
}

func subscriptionShouldNotBeVolatile(groupID string) error {
	subs, _ := w.db.ListSubscriptions()
	for _, s := range subs {
		if s.GroupID == groupID {
			if s.Volatile {
				return fmt.Errorf("subscription %s should not be volatile", groupID)
			}
			return nil
		}
	}
	return fmt.Errorf("subscription %s not found", groupID)
}

func groupShouldHaveNMembers(groupID string, n int) error {
	members := w.mgr.HostedGroupMembers(groupID)
	if len(members) != n {
		return fmt.Errorf("expected %d members in %s, got %d", n, groupID, len(members))
	}
	return nil
}

// ── Suite ──────────────────────────────────────────────────────────────────

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.After(func(ctx2 context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		cleanup()
		return ctx2, nil
	})

	ctx.Step(`^a fresh database$`, aFreshDatabase)
	ctx.Step(`^a registered volatile type "([^"]*)"$`, aRegisteredVolatileType)
	ctx.Step(`^I have a subscription to group "([^"]*)" named "([^"]*)" of type "([^"]*)" from host "([^"]*)"$`, iHaveASubscription)
	ctx.Step(`^I have an active connection to group "([^"]*)" hosted by "([^"]*)" of type "([^"]*)"$`, iHaveAnActiveConnection)
	ctx.Step(`^I host a group "([^"]*)" named "([^"]*)" of type "([^"]*)"$`, iHostAGroup)
	ctx.Step(`^I host a group "([^"]*)" named "([^"]*)" of type "([^"]*)" with max (\d+) members$`, iHostAGroupWithMax)
	ctx.Step(`^I have joined my own group "([^"]*)"$`, iHaveJoinedMyOwnGroup)
	ctx.Step(`^remote peer "([^"]*)" joins group "([^"]*)"$`, remotePeerJoinsGroup)

	ctx.Step(`^I receive an invite for group "([^"]*)" named "([^"]*)" of type "([^"]*)" from host "([^"]*)"$`, iReceiveAnInvite)
	ctx.Step(`^the host closes group "([^"]*)"$`, theHostClosesGroup)
	ctx.Step(`^remote peer "([^"]*)" leaves group "([^"]*)"$`, remotePeerLeavesGroup)

	ctx.Step(`^I should have (\d+) subscriptions?$`, iShouldHaveNSubscriptions)
	ctx.Step(`^subscription "([^"]*)" should have name "([^"]*)"$`, subscriptionShouldHaveName)
	ctx.Step(`^subscription "([^"]*)" should have host "([^"]*)"$`, subscriptionShouldHaveHost)
	ctx.Step(`^subscription "([^"]*)" should not be volatile$`, subscriptionShouldNotBeVolatile)
	ctx.Step(`^group "([^"]*)" should have (\d+) members?$`, groupShouldHaveNMembers)
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog tests failed")
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

type volatileHandler struct{}

func (h *volatileHandler) Flags() group.GroupTypeFlags {
	return group.GroupTypeFlags{HostCanJoin: false, Volatile: true}
}
func (h *volatileHandler) OnCreate(_, _ string, _ int) error { return nil }
func (h *volatileHandler) OnJoin(_, _ string, _ bool)        {}
func (h *volatileHandler) OnLeave(_, _ string, _ bool)       {}
func (h *volatileHandler) OnClose(_ string)                  {}
func (h *volatileHandler) OnEvent(_ *group.Event)            {}
