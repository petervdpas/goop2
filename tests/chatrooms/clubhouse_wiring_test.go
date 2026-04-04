package chatrooms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/cucumber/godog"

	"github.com/petervdpas/goop2/internal/sdk"
)

type sdkWorld struct {
	server   *httptest.Server
	lastResp *http.Response
	lastBody []byte
}

type topicParseResult struct {
	groupID string
	action  string
	valid   bool
}

var sw *sdkWorld
var tp *topicParseResult

func iRequestSDKFile(file string) error {
	if sw == nil {
		sw = &sdkWorld{
			server: httptest.NewServer(http.StripPrefix("/sdk/", sdk.Handler())),
		}
	}
	resp, err := http.Get(sw.server.URL + "/sdk/" + file)
	if err != nil {
		return err
	}
	sw.lastResp = resp
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	sw.lastBody = body
	return nil
}

func theSDKResponseStatusShouldBe(expected int) error {
	if sw.lastResp.StatusCode != expected {
		return errorf("expected SDK status %d, got %d", expected, sw.lastResp.StatusCode)
	}
	return nil
}

func theSDKContentTypeShouldBe(expected string) error {
	ct := sw.lastResp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, expected) {
		return errorf("expected content type %q, got %q", expected, ct)
	}
	return nil
}

func theSDKMQIsLoaded() error {
	return nil
}

func mqSubscribeShouldAcceptTopicAndCallback() error {
	src, err := os.ReadFile("../../internal/sdk/goop-mq.js")
	if err != nil {
		return err
	}
	s := string(src)
	if !strings.Contains(s, "subscribe(topic, fn)") && !strings.Contains(s, "subscribe(topic,fn)") {
		return errorf("goop-mq.js subscribe does not accept (topic, fn) signature")
	}
	return nil
}

func matchTopic(pattern, topic string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(topic, pattern[:len(pattern)-1])
	}
	return pattern == topic
}

func topicShouldMatch(pattern, topic string) error {
	if !matchTopic(pattern, topic) {
		return errorf("expected pattern %q to match topic %q", pattern, topic)
	}
	return nil
}

func topicShouldNotMatch(pattern, topic string) error {
	if matchTopic(pattern, topic) {
		return errorf("expected pattern %q NOT to match topic %q", pattern, topic)
	}
	return nil
}

func theChatroomTopicParserIsLoaded() error {
	tp = nil
	return nil
}

func iParseTopic(topic string) error {
	prefix := "chat.room:"
	if !strings.HasPrefix(topic, prefix) {
		tp = &topicParseResult{valid: false}
		return nil
	}
	rest := topic[len(prefix):]
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		tp = &topicParseResult{valid: false}
		return nil
	}
	tp = &topicParseResult{
		groupID: rest[:idx],
		action:  rest[idx+1:],
		valid:   true,
	}
	return nil
}

func theParsedGroupIDShouldBe(expected string) error {
	if tp == nil || !tp.valid {
		return errorf("topic was not parsed successfully")
	}
	if tp.groupID != expected {
		return errorf("expected group ID %q, got %q", expected, tp.groupID)
	}
	return nil
}

func theParsedActionShouldBe(expected string) error {
	if tp == nil || !tp.valid {
		return errorf("topic was not parsed successfully")
	}
	if tp.action != expected {
		return errorf("expected action %q, got %q", expected, tp.action)
	}
	return nil
}

func theParsedTopicShouldBeInvalid() error {
	if tp == nil {
		return errorf("no parse result")
	}
	if tp.valid {
		return errorf("expected invalid parse, but got groupID=%q action=%q", tp.groupID, tp.action)
	}
	return nil
}

func theClubhouseManifest() error {
	return nil
}

func theManifestNameShouldBe(expected string) error {
	data, err := os.ReadFile("../../internal/sitetemplates/clubhouse/manifest.json")
	if err != nil {
		return err
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return errorf("invalid manifest JSON")
	}
	name, _ := m["name"].(string)
	if name != expected {
		return errorf("expected manifest name %q, got %q", expected, name)
	}
	return nil
}

func theManifestSchemasShouldContain(expected string) error {
	data, err := os.ReadFile("../../internal/sitetemplates/clubhouse/manifest.json")
	if err != nil {
		return err
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return errorf("invalid manifest JSON")
	}
	schemas, _ := m["schemas"].([]any)
	for _, s := range schemas {
		if str, ok := s.(string); ok && str == expected {
			return nil
		}
	}
	return errorf("manifest schemas does not contain %q", expected)
}

func theRoomsSchema() error {
	return nil
}

func theSchemaShouldHaveColumn(colName string) error {
	data, err := os.ReadFile("../../internal/sitetemplates/clubhouse/schemas/rooms.json")
	if err != nil {
		return err
	}
	var schema struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
	}
	if json.Unmarshal(data, &schema) != nil {
		return errorf("invalid schema JSON")
	}
	for _, c := range schema.Columns {
		if c.Name == colName {
			return nil
		}
	}
	return errorf("schema does not have column %q", colName)
}

func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func initClubhouseWiringScenario(ctx *godog.ScenarioContext) {
	ctx.After(func(ctx2 context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if sw != nil {
			sw.server.Close()
			sw = nil
		}
		tp = nil
		return ctx2, nil
	})

	ctx.Step(`^I request SDK file "([^"]*)"$`, iRequestSDKFile)
	ctx.Step(`^the SDK response status should be (\d+)$`, theSDKResponseStatusShouldBe)
	ctx.Step(`^the SDK content type should be "([^"]*)"$`, theSDKContentTypeShouldBe)

	ctx.Step(`^the SDK MQ is loaded$`, theSDKMQIsLoaded)
	ctx.Step(`^Goop\.mq\.subscribe should accept a topic and callback$`, mqSubscribeShouldAcceptTopicAndCallback)
	ctx.Step(`^topic "([^"]*)" should match "([^"]*)"$`, topicShouldMatch)
	ctx.Step(`^topic "([^"]*)" should not match "([^"]*)"$`, topicShouldNotMatch)

	ctx.Step(`^the chatroom topic parser is loaded$`, theChatroomTopicParserIsLoaded)
	ctx.Step(`^I parse topic "([^"]*)"$`, iParseTopic)
	ctx.Step(`^the parsed group ID should be "([^"]*)"$`, theParsedGroupIDShouldBe)
	ctx.Step(`^the parsed action should be "([^"]*)"$`, theParsedActionShouldBe)
	ctx.Step(`^the parsed topic should be invalid$`, theParsedTopicShouldBeInvalid)

	ctx.Step(`^the clubhouse manifest$`, theClubhouseManifest)
	ctx.Step(`^the manifest name should be "([^"]*)"$`, theManifestNameShouldBe)
	ctx.Step(`^the manifest schemas should contain "([^"]*)"$`, theManifestSchemasShouldContain)
	ctx.Step(`^the rooms schema$`, theRoomsSchema)
	ctx.Step(`^the schema should have column "([^"]*)"$`, theSchemaShouldHaveColumn)
}
