package chatrooms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cucumber/godog"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/group_types/chat"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/viewer/routes"
)

type world struct {
	server   *httptest.Server
	chatMgr  *chat.Manager
	grpMgr   *group.Manager
	db       *storage.DB
	dir      string
	lastResp *http.Response
	lastBody []byte
	lastRoom string
}

var w *world

func aRunningChatRoomServer() error {
	dir := ""
	var err error
	dir, err = createTempDir()
	if err != nil {
		return err
	}
	db, err := storage.Open(dir)
	if err != nil {
		return err
	}

	grpMgr := group.NewTestManager(db, "self-peer-id")

	cm := newTestChatManager(grpMgr)

	mux := http.NewServeMux()
	routes.RegisterChatRooms(mux, cm, func(id string) string {
		if id == "self-peer-id" {
			return "Self"
		}
		return id
	})

	srv := httptest.NewServer(mux)

	w = &world{
		server:  srv,
		chatMgr: cm,
		grpMgr:  grpMgr,
		db:      db,
		dir:     dir,
	}
	return nil
}

func cleanup() {
	if w != nil {
		w.server.Close()
		w.grpMgr.Close()
		w.db.Close()
		removeTempDir(w.dir)
		w = nil
	}
}

func iCreateARoom(name, description string, maxMembers int) error {
	body, _ := json.Marshal(map[string]any{
		"name":        name,
		"description": description,
		"max_members": maxMembers,
	})
	resp, err := http.Post(w.server.URL+"/api/chat/rooms/create", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return captureResponse(resp)
}

func iHaveCreatedARoom(name string) error {
	if err := iCreateARoom(name, "", 0); err != nil {
		return err
	}
	if w.lastResp.StatusCode != 200 {
		return fmt.Errorf("setup: create room returned %d", w.lastResp.StatusCode)
	}
	var room struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.lastBody, &room); err != nil {
		return err
	}
	w.lastRoom = room.ID
	return nil
}

func iRequestTheState() error {
	resp, err := http.Get(w.server.URL + "/api/chat/rooms/state?group_id=" + w.lastRoom)
	if err != nil {
		return err
	}
	return captureResponse(resp)
}

func iSendMessageToThatRoom(text string) error {
	return sendMessage(w.lastRoom, text)
}

func iSendMessageToRoom(text, roomID string) error {
	return sendMessage(roomID, text)
}

func sendMessage(roomID, text string) error {
	body, _ := json.Marshal(map[string]any{
		"group_id": roomID,
		"text":     text,
	})
	resp, err := http.Post(w.server.URL+"/api/chat/rooms/send", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return captureResponse(resp)
}

func iCloseThatRoom() error {
	body, _ := json.Marshal(map[string]any{
		"group_id": w.lastRoom,
	})
	resp, err := http.Post(w.server.URL+"/api/chat/rooms/close", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return captureResponse(resp)
}

func theResponseStatusShouldBe(expected int) error {
	if w.lastResp.StatusCode != expected {
		return fmt.Errorf("expected status %d, got %d (body: %s)", expected, w.lastResp.StatusCode, string(w.lastBody))
	}
	return nil
}

func theResponseShouldContainRoomName(name string) error {
	var room struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.lastBody, &room); err != nil {
		return err
	}
	if room.Name != name {
		return fmt.Errorf("expected room name %q, got %q", name, room.Name)
	}
	return nil
}

func theStateShouldHaveNMessages(n int) error {
	var state struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(w.lastBody, &state); err != nil {
		return err
	}
	if len(state.Messages) != n {
		return fmt.Errorf("expected %d messages, got %d", n, len(state.Messages))
	}
	return nil
}

func theLatestMessageTextShouldBe(expected string) error {
	var state struct {
		Messages []struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(w.lastBody, &state); err != nil {
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

func captureResponse(resp *http.Response) error {
	w.lastResp = resp
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	w.lastBody = body
	return nil
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.After(func(ctx2 context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		cleanup()
		return ctx2, nil
	})

	ctx.Step(`^a running chat room server$`, aRunningChatRoomServer)

	ctx.Step(`^I create a room named "([^"]*)" with description "([^"]*)" and max members (\d+)$`, iCreateARoom)
	ctx.Step(`^I have created a room named "([^"]*)"$`, iHaveCreatedARoom)
	ctx.Step(`^I request the state of that room$`, iRequestTheState)
	ctx.Step(`^I send message "([^"]*)" to that room$`, iSendMessageToThatRoom)
	ctx.Step(`^I send message "([^"]*)" to room "([^"]*)"$`, iSendMessageToRoom)
	ctx.Step(`^I close that room$`, iCloseThatRoom)

	ctx.Step(`^the response status should be (\d+)$`, theResponseStatusShouldBe)
	ctx.Step(`^the response should contain room name "([^"]*)"$`, theResponseShouldContainRoomName)
	ctx.Step(`^the state should have (\d+) messages?$`, theStateShouldHaveNMessages)
	ctx.Step(`^the latest message text should be "([^"]*)"$`, theLatestMessageTextShouldBe)
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			InitializeScenario(ctx)
			initClubhouseWiringScenario(ctx)
			initJoinerScenario(ctx)
		},
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

func newTestChatManager(grpMgr *group.Manager) *chat.Manager {
	return chat.NewTestManager(grpMgr, "self-peer-id", func(id string) string {
		if id == "self-peer-id" {
			return "Self"
		}
		return id
	})
}

func createTempDir() (string, error) {
	return os.MkdirTemp("", "godog-chatrooms-*")
}

func removeTempDir(dir string) {
	os.RemoveAll(dir)
}
