package chatrooms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/cucumber/godog"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/group_types/chat"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/viewer/routes"
)

type joinerWorld struct {
	hostDir     string
	joinerDir   string
	hostDB      *storage.DB
	joinerDB    *storage.DB
	hostGrp     *group.Manager
	joinerGrp   *group.Manager
	hostChat    *chat.Manager
	joinerChat  *chat.Manager
	hostServer  *httptest.Server
	joinerServer *httptest.Server

	lastHostResp   *http.Response
	lastHostBody   []byte
	lastJoinerResp *http.Response
	lastJoinerBody []byte

	roomGroupIDs map[string]string // room name → group ID
}

var jw *joinerWorld

func aRunningHostAndJoinerChatServer() error {
	hostDir, err := createTempDir()
	if err != nil {
		return err
	}
	joinerDir, err := createTempDir()
	if err != nil {
		return err
	}

	hostDB, err := storage.Open(hostDir)
	if err != nil {
		return err
	}
	joinerDB, err := storage.Open(joinerDir)
	if err != nil {
		return err
	}

	hostGrp := group.NewTestManager(hostDB, "host-peer-id")
	joinerGrp := group.NewTestManager(joinerDB, "joiner-peer")

	peerName := func(id string) string {
		switch id {
		case "host-peer-id":
			return "Host"
		case "joiner-peer":
			return "Joiner"
		}
		return id
	}

	hostChat := chat.NewTestManager(hostGrp, "host-peer-id", peerName)
	joinerChat := chat.NewTestManager(joinerGrp, "joiner-peer", peerName)

	hostMux := http.NewServeMux()
	routes.RegisterChatRooms(hostMux, hostChat, peerName)
	hostSrv := httptest.NewServer(hostMux)

	joinerMux := http.NewServeMux()
	routes.RegisterChatRooms(joinerMux, joinerChat, peerName)
	joinerSrv := httptest.NewServer(joinerMux)

	jw = &joinerWorld{
		hostDir:      hostDir,
		joinerDir:    joinerDir,
		hostDB:       hostDB,
		joinerDB:     joinerDB,
		hostGrp:      hostGrp,
		joinerGrp:    joinerGrp,
		hostChat:     hostChat,
		joinerChat:   joinerChat,
		hostServer:   hostSrv,
		joinerServer: joinerSrv,
		roomGroupIDs: make(map[string]string),
	}
	return nil
}

func joinerCleanup() {
	if jw != nil {
		jw.hostServer.Close()
		jw.joinerServer.Close()
		jw.hostGrp.Close()
		jw.joinerGrp.Close()
		jw.hostDB.Close()
		jw.joinerDB.Close()
		removeTempDir(jw.hostDir)
		removeTempDir(jw.joinerDir)
		jw = nil
	}
}

func theHostHasCreatedRoom(name, description string) error {
	room, err := jw.hostChat.CreateRoom(name, description, "", 0)
	if err != nil {
		return err
	}
	jw.roomGroupIDs[name] = room.ID
	return nil
}

func peerHasJoinedTheHostGroupFor(peerID, roomName string) error {
	groupID, ok := jw.roomGroupIDs[roomName]
	if !ok {
		return fmt.Errorf("room %q not found", roomName)
	}
	jw.hostGrp.SimulateJoin(peerID, groupID)
	jw.joinerGrp.SetActiveConn(groupID, "host-peer-id", chat.GroupTypeName)
	jw.joinerGrp.SetActiveConnMembers(groupID, []group.MemberInfo{
		{PeerID: "host-peer-id", Name: "Host"},
		{PeerID: peerID, Name: "Joiner"},
	})
	jw.joinerChat.RegisterJoinedRoom(groupID, roomName)
	return nil
}

func peerHasLeftTheHostGroupFor(peerID, roomName string) error {
	groupID, ok := jw.roomGroupIDs[roomName]
	if !ok {
		return fmt.Errorf("room %q not found", roomName)
	}
	jw.hostGrp.SimulateLeave(peerID, groupID)
	return nil
}

func theHostHasJoinedItsOwnGroupFor(roomName string) error {
	groupID, ok := jw.roomGroupIDs[roomName]
	if !ok {
		return fmt.Errorf("room %q not found", roomName)
	}
	return jw.hostGrp.JoinOwnGroup(groupID)
}

func theJoinerRequestsTheStateOf(roomName string) error {
	groupID, ok := jw.roomGroupIDs[roomName]
	if !ok {
		return fmt.Errorf("room %q not found", roomName)
	}
	resp, err := http.Get(jw.joinerServer.URL + "/api/chat/rooms/state?group_id=" + groupID)
	if err != nil {
		return err
	}
	return captureJoinerResponse(resp)
}

func theHostRequestsTheStateOf(roomName string) error {
	groupID, ok := jw.roomGroupIDs[roomName]
	if !ok {
		return fmt.Errorf("room %q not found", roomName)
	}
	resp, err := http.Get(jw.hostServer.URL + "/api/chat/rooms/state?group_id=" + groupID)
	if err != nil {
		return err
	}
	return captureHostResponse(resp)
}

func theJoinerSendsMessageTo(text, roomName string) error {
	groupID, ok := jw.roomGroupIDs[roomName]
	if !ok {
		return fmt.Errorf("room %q not found", roomName)
	}
	body, _ := json.Marshal(map[string]any{
		"group_id": groupID,
		"text":     text,
	})
	resp, err := http.Post(jw.joinerServer.URL+"/api/chat/rooms/send", "application/json",
		 bytes.NewReader(body))
	if err != nil {
		return err
	}
	return captureJoinerResponse(resp)
}

func theJoinerResponseStatusShouldBe(expected int) error {
	if jw.lastJoinerResp.StatusCode != expected {
		return fmt.Errorf("expected joiner status %d, got %d (body: %s)",
			expected, jw.lastJoinerResp.StatusCode, string(jw.lastJoinerBody))
	}
	return nil
}

func theJoinerStateShouldHaveRoomName(name string) error {
	var state struct {
		Room struct {
			Name string `json:"name"`
		} `json:"room"`
	}
	if err := json.Unmarshal(jw.lastJoinerBody, &state); err != nil {
		return err
	}
	if state.Room.Name != name {
		return fmt.Errorf("expected room name %q, got %q", name, state.Room.Name)
	}
	return nil
}

func theJoinerStateShouldHaveAtLeastNMembers(n int) error {
	var state struct {
		Room struct {
			Members []json.RawMessage `json:"members"`
		} `json:"room"`
	}
	if err := json.Unmarshal(jw.lastJoinerBody, &state); err != nil {
		return err
	}
	if len(state.Room.Members) < n {
		return fmt.Errorf("expected at least %d members, got %d", n, len(state.Room.Members))
	}
	return nil
}

func theJoinerStateShouldHaveNMessages(n int) error {
	var state struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(jw.lastJoinerBody, &state); err != nil {
		return err
	}
	if len(state.Messages) != n {
		return fmt.Errorf("expected %d messages, got %d", n, len(state.Messages))
	}
	return nil
}

func theJoinerLatestMessageTextShouldBe(expected string) error {
	var state struct {
		Messages []struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(jw.lastJoinerBody, &state); err != nil {
		return err
	}
	if len(state.Messages) == 0 {
		return fmt.Errorf("no messages found")
	}
	last := state.Messages[len(state.Messages)-1]
	if last.Text != expected {
		return fmt.Errorf("expected latest text %q, got %q", expected, last.Text)
	}
	return nil
}

func theHostStateShouldHaveNMembers(n int) error {
	var state struct {
		Room struct {
			Members []json.RawMessage `json:"members"`
		} `json:"room"`
	}
	if err := json.Unmarshal(jw.lastHostBody, &state); err != nil {
		return err
	}
	if len(state.Room.Members) != n {
		return fmt.Errorf("expected %d members, got %d", n, len(state.Room.Members))
	}
	return nil
}

func captureJoinerResponse(resp *http.Response) error {
	jw.lastJoinerResp = resp
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	jw.lastJoinerBody = body
	return nil
}

func captureHostResponse(resp *http.Response) error {
	jw.lastHostResp = resp
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	jw.lastHostBody = body
	return nil
}

func initJoinerScenario(ctx *godog.ScenarioContext) {
	ctx.After(func(ctx2 context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		joinerCleanup()
		return ctx2, nil
	})

	ctx.Step(`^a running host and joiner chat server$`, aRunningHostAndJoinerChatServer)
	ctx.Step(`^the host has created room "([^"]*)" with description "([^"]*)"$`, theHostHasCreatedRoom)
	ctx.Step(`^peer "([^"]*)" has joined the host group for "([^"]*)"$`, peerHasJoinedTheHostGroupFor)
	ctx.Step(`^peer "([^"]*)" has left the host group for "([^"]*)"$`, peerHasLeftTheHostGroupFor)
	ctx.Step(`^the host has joined its own group for "([^"]*)"$`, theHostHasJoinedItsOwnGroupFor)

	ctx.Step(`^the joiner requests the state of "([^"]*)"$`, theJoinerRequestsTheStateOf)
	ctx.Step(`^the host requests the state of "([^"]*)"$`, theHostRequestsTheStateOf)
	ctx.Step(`^the joiner sends message "([^"]*)" to "([^"]*)"$`, theJoinerSendsMessageTo)

	ctx.Step(`^the joiner response status should be (\d+)$`, theJoinerResponseStatusShouldBe)
	ctx.Step(`^the joiner state should have room name "([^"]*)"$`, theJoinerStateShouldHaveRoomName)
	ctx.Step(`^the joiner state should have at least (\d+) members?$`, theJoinerStateShouldHaveAtLeastNMembers)
	ctx.Step(`^the joiner state should have (\d+) messages?$`, theJoinerStateShouldHaveNMessages)
	ctx.Step(`^the joiner latest message text should be "([^"]*)"$`, theJoinerLatestMessageTextShouldBe)
	ctx.Step(`^the host state should have (\d+) members?$`, theHostStateShouldHaveNMembers)
}
