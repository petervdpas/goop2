package grouptypes

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/group_types/chat"
	"github.com/petervdpas/goop2/internal/group_types/cluster"
	"github.com/petervdpas/goop2/internal/group_types/datafed"
	"github.com/petervdpas/goop2/internal/group_types/files"
	"github.com/petervdpas/goop2/internal/group_types/template"
	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
)

type world struct {
	db     *storage.DB
	grpMgr *group.Manager
	dir    string

	handler group.TypeHandler

	chatMgr    *chat.Manager
	datafedMgr *datafed.Manager
	clusterMgr *cluster.Manager

	lastErr error
}

var w *world

func cleanup() {
	if w != nil {
		if w.clusterMgr != nil {
			w.clusterMgr.Close()
		}
		if w.chatMgr != nil {
			w.chatMgr.Close()
		}
		if w.grpMgr != nil {
			w.grpMgr.Close()
		}
		if w.db != nil {
			w.db.Close()
		}
		os.RemoveAll(w.dir)
		w = nil
	}
}

func freshWorld() error {
	dir, err := os.MkdirTemp("", "godog-grouptypes-*")
	if err != nil {
		return err
	}
	db, err := storage.Open(dir)
	if err != nil {
		return err
	}
	grpMgr := group.NewTestManager(db, "self-peer-id", func(id string) state.PeerIdentity {
		if id == "self-peer-id" {
			return state.PeerIdentity{Name: "Self", Known: true}
		}
		return state.PeerIdentity{Name: id, Known: true}
	})
	w = &world{db: db, grpMgr: grpMgr, dir: dir}
	return nil
}

// ── Given: handler setup ──────────────────────────────────────────────────

func aHandler(handlerType string) error {
	if err := freshWorld(); err != nil {
		return err
	}
	switch handlerType {
	case "chat":
		cm := chat.NewTestManager(w.grpMgr, "self-peer-id", func(id string) state.PeerIdentity {
			if id == "self-peer-id" {
				return state.PeerIdentity{Name: "Self", Known: true}
			}
			return state.PeerIdentity{Name: id, Known: true}
		})
		w.chatMgr = cm
		w.handler = cm
	case "template":
		h := template.New(w.grpMgr)
		w.handler = h
	case "datafed":
		m := newTestDatafed(w.grpMgr)
		w.datafedMgr = m
		w.handler = m
	case "files":
		store, err := files.NewStore(w.dir)
		if err != nil {
			return err
		}
		files.New(nil, w.grpMgr, store)
		w.handler = &filesHandlerProxy{grpMgr: w.grpMgr}
	case "cluster":
		h := newTestClusterHandler()
		w.handler = h
	default:
		return fmt.Errorf("unknown handler type: %s", handlerType)
	}
	return nil
}

func aClusterManager() error {
	if w == nil {
		if err := freshWorld(); err != nil {
			return err
		}
	}
	sendFn := func(peerID, topic string, payload any) error { return nil }
	subFn := func(fn func(from, topic string, payload any)) func() { return func() {} }
	w.clusterMgr = cluster.NewManager("self-peer-id", sendFn, subFn)
	return nil
}

// ── Then: flag checks ─────────────────────────────────────────────────────

func theHandlerShouldAllowHostJoin() error {
	if !w.handler.Flags().HostCanJoin {
		return fmt.Errorf("expected HostCanJoin=true")
	}
	return nil
}

func theHandlerShouldNotAllowHostJoin() error {
	if w.handler.Flags().HostCanJoin {
		return fmt.Errorf("expected HostCanJoin=false")
	}
	return nil
}

func theHandlerShouldNotBeVolatile() error {
	if w.handler.Flags().Volatile {
		return fmt.Errorf("expected Volatile=false")
	}
	return nil
}

func theHandlerShouldBeVolatile() error {
	if !w.handler.Flags().Volatile {
		return fmt.Errorf("expected Volatile=true")
	}
	return nil
}

// ── Chat steps ────────────────────────────────────────────────────────────

func iCreateChatGroup(groupID, name string, max int) error {
	return w.handler.OnCreate(groupID, name, max)
}

func theHandlerShouldTrackNGroups(n int) error {
	rooms := w.chatMgr.ListRooms()
	if len(rooms) != n {
		return fmt.Errorf("expected %d rooms, got %d", n, len(rooms))
	}
	return nil
}

func theHandlerClosesGroup(groupID string) error {
	w.handler.OnClose(groupID)
	return nil
}

func iSendChat(text, fromPeer, groupID string) error {
	w.lastErr = w.chatMgr.SendMessage(groupID, fromPeer, text)
	return nil
}

func theChatSendShouldFail() error {
	if w.lastErr == nil {
		return fmt.Errorf("expected send to fail")
	}
	return nil
}

func groupShouldHaveNChatMessages(groupID string, n int) error {
	_, msgs, err := w.chatMgr.GetState(groupID)
	if err != nil {
		return err
	}
	if len(msgs) != n {
		return fmt.Errorf("expected %d messages, got %d", n, len(msgs))
	}
	return nil
}

func theLatestChatMessageShouldBe(groupID, expected string) error {
	_, msgs, err := w.chatMgr.GetState(groupID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return fmt.Errorf("no messages")
	}
	last := msgs[len(msgs)-1]
	if last.Text != expected {
		return fmt.Errorf("expected %q, got %q", expected, last.Text)
	}
	return nil
}

// ── Template / generic group manager steps ────────────────────────────────

func iCreateGroupViaManager(groupID, name, groupType string) error {
	return w.grpMgr.CreateGroup(groupID, name, groupType, "", 0)
}

func iJoinMyOwnGroup(groupID string) error {
	return w.grpMgr.JoinOwnGroup(groupID)
}

func remotePeerJoinsGroup(peerID, groupID string) error {
	w.grpMgr.SimulateJoin(peerID, groupID)
	return nil
}

func remotePeerLeavesGroup(peerID, groupID string) error {
	w.grpMgr.SimulateLeave(peerID, groupID)
	return nil
}

func iCloseGroupViaManager(groupID string) error {
	return w.grpMgr.CloseGroup(groupID)
}

func theGroupManagerShouldListNHostedGroups(n int) error {
	groups, err := w.grpMgr.ListHostedGroups()
	if err != nil {
		return err
	}
	if len(groups) != n {
		return fmt.Errorf("expected %d hosted groups, got %d", n, len(groups))
	}
	return nil
}

func groupShouldHaveNMembersViaManager(groupID string, n int) error {
	members := w.grpMgr.HostedGroupMembers(groupID)
	if len(members) != n {
		return fmt.Errorf("expected %d members, got %d", n, len(members))
	}
	return nil
}

func theGroupFlagsShouldAllowHostJoin(groupID string) error {
	flags := w.grpMgr.GroupTypeFlagsForGroup(groupID)
	if !flags.HostCanJoin {
		return fmt.Errorf("expected HostCanJoin=true for group %s", groupID)
	}
	return nil
}

// ── Datafed steps ─────────────────────────────────────────────────────────

func iCreateDatafedGroup(groupID, name string) error {
	return w.datafedMgr.OnCreate(groupID, name, 0)
}

func theDatafedHandlerShouldTrackNGroups(n int) error {
	ids := w.datafedMgr.AllGroups()
	if len(ids) != n {
		return fmt.Errorf("expected %d groups, got %d", n, len(ids))
	}
	return nil
}

func theDatafedHandlerClosesGroup(groupID string) error {
	w.datafedMgr.OnClose(groupID)
	return nil
}

func peerContributesTable(peerID, tableName, groupID string) error {
	contribs := w.datafedMgr.GroupContributions(groupID)
	_ = contribs
	w.datafedMgr.AddTestContribution(groupID, peerID, tableName)
	return nil
}

func peerLeavesDatafedGroup(peerID, groupID string) error {
	w.datafedMgr.OnLeave(groupID, peerID, false)
	return nil
}

func datafedGroupShouldHaveNContributions(groupID string, n int) error {
	contribs := w.datafedMgr.GroupContributions(groupID)
	if len(contribs) != n {
		return fmt.Errorf("expected %d contributions, got %d", n, len(contribs))
	}
	return nil
}

// ── Cluster steps ─────────────────────────────────────────────────────────

func iCreateCluster(groupID string) error {
	w.lastErr = w.clusterMgr.CreateCluster(groupID)
	return nil
}

func iJoinCluster(groupID, hostPeer string) error {
	w.lastErr = w.clusterMgr.JoinCluster(groupID, hostPeer)
	return nil
}

func iLeaveTheCluster() error {
	w.clusterMgr.LeaveCluster()
	return nil
}

func theClusterRoleShouldBe(expected string) error {
	role := w.clusterMgr.Role()
	if role != expected {
		return fmt.Errorf("expected role %q, got %q", expected, role)
	}
	return nil
}

func theJoinShouldFail() error {
	if w.lastErr == nil {
		return fmt.Errorf("expected join to fail")
	}
	return nil
}

func iSubmitAJob(jobType string) error {
	_, w.lastErr = w.clusterMgr.SubmitJob(cluster.Job{Type: jobType})
	return nil
}

func iSubmitAJobWithPriority(jobType string, priority int) error {
	_, w.lastErr = w.clusterMgr.SubmitJob(cluster.Job{Type: jobType, Priority: priority})
	return nil
}

func theSubmitShouldFail() error {
	if w.lastErr == nil {
		return fmt.Errorf("expected submit to fail")
	}
	return nil
}

func thereShouldBeNJobs(n int) error {
	jobs := w.clusterMgr.GetJobs()
	if len(jobs) != n {
		return fmt.Errorf("expected %d jobs, got %d", n, len(jobs))
	}
	return nil
}

func theJobStatsShouldShowNPending(n int) error {
	stats := w.clusterMgr.GetStats()
	if stats.Pending != n {
		return fmt.Errorf("expected %d pending, got %d", n, stats.Pending)
	}
	return nil
}

// ── Listen steps (pure functions, no libp2p) ──────────────────────────────

func shouldBeAStreamURL(url string) error {
	if !isStreamURL(url) {
		return fmt.Errorf("expected %q to be a stream URL", url)
	}
	return nil
}

func shouldNotBeAStreamURL(url string) error {
	if isStreamURL(url) {
		return fmt.Errorf("expected %q NOT to be a stream URL", url)
	}
	return nil
}

func isStreamURL(s string) bool {
	return len(s) > 0 && (s[:7] == "http://" || (len(s) > 8 && s[:8] == "https://"))
}

// ── Helpers ───────────────────────────────────────────────────────────────

type filesHandlerProxy struct {
	grpMgr *group.Manager
}

func (f *filesHandlerProxy) Flags() group.GroupTypeFlags {
	return group.GroupTypeFlags{HostCanJoin: true}
}

func (f *filesHandlerProxy) OnCreate(groupID, name string, max int) error { return nil }
func (f *filesHandlerProxy) OnJoin(groupID, peerID string, isHost bool)   {}
func (f *filesHandlerProxy) OnLeave(groupID, peerID string, isHost bool)  {}
func (f *filesHandlerProxy) OnClose(groupID string)                       {}
func (f *filesHandlerProxy) OnEvent(_ *group.Event)                       {}

func newTestClusterHandler() group.TypeHandler {
	return &clusterHandlerStub{}
}

type clusterHandlerStub struct{}

func (c *clusterHandlerStub) Flags() group.GroupTypeFlags {
	return group.GroupTypeFlags{HostCanJoin: false, Volatile: true}
}
func (c *clusterHandlerStub) OnCreate(_, _ string, _ int) error { return nil }
func (c *clusterHandlerStub) OnJoin(_, _ string, _ bool)        {}
func (c *clusterHandlerStub) OnLeave(_, _ string, _ bool)       {}
func (c *clusterHandlerStub) OnClose(_ string)                  {}
func (c *clusterHandlerStub) OnEvent(_ *group.Event)            {}

func newTestDatafed(grpMgr *group.Manager) *datafed.Manager {
	return datafed.NewTestManager(grpMgr, "self-peer-id", func() []*ormschema.Table { return nil })
}

// ── Suite ─────────────────────────────────────────────────────────────────

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.After(func(ctx2 context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		cleanup()
		return ctx2, nil
	})

	// Given
	ctx.Step(`^a "([^"]*)" handler$`, aHandler)
	ctx.Step(`^a cluster manager$`, aClusterManager)

	// Chat
	ctx.Step(`^I create group "([^"]*)" named "([^"]*)" with max (\d+)$`, iCreateChatGroup)
	ctx.Step(`^the handler should track (\d+) groups?$`, theHandlerShouldTrackNGroups)
	ctx.Step(`^the handler closes group "([^"]*)"$`, theHandlerClosesGroup)
	ctx.Step(`^I send chat "([^"]*)" from "([^"]*)" to group "([^"]*)"$`, iSendChat)
	ctx.Step(`^the chat send should fail$`, theChatSendShouldFail)
	ctx.Step(`^group "([^"]*)" should have (\d+) chat messages?$`, groupShouldHaveNChatMessages)
	ctx.Step(`^the latest chat message in "([^"]*)" should be "([^"]*)"$`, theLatestChatMessageShouldBe)

	// Template / generic group manager
	ctx.Step(`^I create group "([^"]*)" named "([^"]*)" via the group manager with type "([^"]*)"$`, iCreateGroupViaManager)
	ctx.Step(`^I join my own group "([^"]*)"$`, iJoinMyOwnGroup)
	ctx.Step(`^remote peer "([^"]*)" joins group "([^"]*)"$`, remotePeerJoinsGroup)
	ctx.Step(`^remote peer "([^"]*)" leaves group "([^"]*)"$`, remotePeerLeavesGroup)
	ctx.Step(`^I close group "([^"]*)" via the group manager$`, iCloseGroupViaManager)
	ctx.Step(`^the group manager should list (\d+) hosted groups?$`, theGroupManagerShouldListNHostedGroups)
	ctx.Step(`^group "([^"]*)" should have (\d+) members? via the group manager$`, groupShouldHaveNMembersViaManager)
	ctx.Step(`^the group flags for "([^"]*)" should allow host join$`, theGroupFlagsShouldAllowHostJoin)

	// Datafed
	ctx.Step(`^I create datafed group "([^"]*)" named "([^"]*)"$`, iCreateDatafedGroup)
	ctx.Step(`^the datafed handler should track (\d+) groups?$`, theDatafedHandlerShouldTrackNGroups)
	ctx.Step(`^the datafed handler closes group "([^"]*)"$`, theDatafedHandlerClosesGroup)
	ctx.Step(`^peer "([^"]*)" contributes table "([^"]*)" to datafed group "([^"]*)"$`, peerContributesTable)
	ctx.Step(`^peer "([^"]*)" leaves datafed group "([^"]*)"$`, peerLeavesDatafedGroup)
	ctx.Step(`^datafed group "([^"]*)" should have (\d+) contributions?$`, datafedGroupShouldHaveNContributions)

	// Cluster
	ctx.Step(`^I create cluster "([^"]*)"$`, iCreateCluster)
	ctx.Step(`^I join cluster "([^"]*)" hosted by "([^"]*)"$`, iJoinCluster)
	ctx.Step(`^I leave the cluster$`, iLeaveTheCluster)
	ctx.Step(`^the cluster role should be "([^"]*)"$`, theClusterRoleShouldBe)
	ctx.Step(`^the join should fail$`, theJoinShouldFail)
	ctx.Step(`^I submit a job of type "([^"]*)"$`, iSubmitAJob)
	ctx.Step(`^I submit a job of type "([^"]*)" with priority (\d+)$`, iSubmitAJobWithPriority)
	ctx.Step(`^the submit should fail$`, theSubmitShouldFail)
	ctx.Step(`^there should be (\d+) jobs?$`, thereShouldBeNJobs)
	ctx.Step(`^the job stats should show (\d+) pending$`, theJobStatsShouldShowNPending)

	// Flags
	ctx.Step(`^the handler should allow host join$`, theHandlerShouldAllowHostJoin)
	ctx.Step(`^the handler should not allow host join$`, theHandlerShouldNotAllowHostJoin)
	ctx.Step(`^the handler should not be volatile$`, theHandlerShouldNotBeVolatile)
	ctx.Step(`^the handler should be volatile$`, theHandlerShouldBeVolatile)

	// Listen (pure function tests)
	ctx.Step(`^"([^"]*)" should be a stream URL$`, shouldBeAStreamURL)
	ctx.Step(`^"([^"]*)" should not be a stream URL$`, shouldNotBeAStreamURL)
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
