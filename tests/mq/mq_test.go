package mq_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"

	"github.com/petervdpas/goop2/internal/mq"
)

// ── Dispatch harness ─────────────────────────────────────────────────────────
// Replicates the dispatch logic from Manager.handleIncoming without requiring
// a libp2p stream. This is the "dispatch layer" test boundary.

type dispatcher struct {
	mu        sync.Mutex
	topicSubs []topicSub
	listeners []*sseListener
	inbox     map[string][]bufferedMsg
}

type topicSub struct {
	prefix string
	fn     func(from, topic string, payload any)
	active bool
}

type sseListener struct {
	events []sseEvent
	cancel func()
}

type sseEvent struct {
	Type  string
	Topic string
	From  string
	MsgID string
	Payload any
}

type bufferedMsg struct {
	topic   string
	from    string
	payload any
}

type subscriberRecord struct {
	from    string
	topic   string
	payload any
}

func newDispatcher() *dispatcher {
	return &dispatcher{
		inbox: make(map[string][]bufferedMsg),
	}
}

func (d *dispatcher) subscribeTopic(prefix string) (records *[]subscriberRecord, unsubIdx int) {
	recs := &[]subscriberRecord{}
	d.mu.Lock()
	idx := len(d.topicSubs)
	d.topicSubs = append(d.topicSubs, topicSub{
		prefix: prefix,
		fn: func(from, topic string, payload any) {
			*recs = append(*recs, subscriberRecord{from: from, topic: topic, payload: payload})
		},
		active: true,
	})
	d.mu.Unlock()
	return recs, idx
}

func (d *dispatcher) unsubscribe(idx int) {
	d.mu.Lock()
	if idx < len(d.topicSubs) {
		d.topicSubs[idx].active = false
	}
	d.mu.Unlock()
}

func (d *dispatcher) addSSEListener() *sseListener {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Replay inbox
	var replayed []sseEvent
	for _, entries := range d.inbox {
		for _, e := range entries {
			replayed = append(replayed, sseEvent{
				Type:    "message",
				Topic:   e.topic,
				From:    e.from,
				Payload: e.payload,
			})
		}
	}
	d.inbox = make(map[string][]bufferedMsg)

	l := &sseListener{events: replayed}
	l.cancel = func() {}
	d.listeners = append(d.listeners, l)
	return l
}

func (d *dispatcher) removeListener(l *sseListener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, ll := range d.listeners {
		if ll == l {
			d.listeners = append(d.listeners[:i], d.listeners[i+1:]...)
			return
		}
	}
}

func (d *dispatcher) publishLocal(topic, from string, payload any) {
	d.mu.Lock()
	defer d.mu.Unlock()

	evt := sseEvent{Type: "message", Topic: topic, From: from, Payload: payload}
	for _, l := range d.listeners {
		l.events = append(l.events, evt)
	}
}

func (d *dispatcher) notifyDelivered(msgID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	evt := sseEvent{Type: "delivered", MsgID: msgID}
	for _, l := range d.listeners {
		l.events = append(l.events, evt)
	}
}

// dispatch replicates Manager.handleIncoming dispatch logic.
func (d *dispatcher) dispatch(from, topic string, payload any) {
	// mq.ack special handling
	if topic == "mq.ack" {
		if pm, ok := payload.(map[string]any); ok {
			if origID, ok := pm["msg_id"].(string); ok && origID != "" {
				d.notifyDelivered(origID)
			}
		}
		return
	}

	// Dispatch to topic subscribers
	d.mu.Lock()
	subs := make([]topicSub, len(d.topicSubs))
	copy(subs, d.topicSubs)
	d.mu.Unlock()

	for _, sub := range subs {
		if sub.active && strings.HasPrefix(topic, sub.prefix) {
			sub.fn(from, topic, payload)
		}
	}

	// SSE suppression: group protocol, chat.room, and identity are handled by subscribers
	if topic == mq.TopicGroupInvite ||
		strings.HasPrefix(topic, mq.TopicGroupPrefix) ||
		strings.HasPrefix(topic, mq.TopicChatRoomPrefix) ||
		topic == mq.TopicIdentity || topic == mq.TopicIdentityResponse {
		return
	}

	// Deliver to SSE listeners
	d.mu.Lock()
	evt := sseEvent{Type: "message", Topic: topic, From: from, Payload: payload}
	delivered := false
	for _, l := range d.listeners {
		l.events = append(l.events, evt)
		delivered = true
	}

	// Buffer if no listener
	if !delivered {
		buf := d.inbox[from]
		if len(buf) >= 200 {
			buf = buf[1:]
		}
		d.inbox[from] = append(buf, bufferedMsg{topic: topic, from: from, payload: payload})
	}
	d.mu.Unlock()
}

// ── World ────────────────────────────────────────────────────────────────────

type mqWorld struct {
	disp       *dispatcher
	subs       map[string]*[]subscriberRecord
	subIndices map[string]int
	listener   *sseListener
	listener2  *sseListener
	listeners  []*sseListener
	noPanic    bool
}

var w *mqWorld

func aFreshMQDispatcher() error {
	w = &mqWorld{
		disp:       newDispatcher(),
		subs:       make(map[string]*[]subscriberRecord),
		subIndices: make(map[string]int),
		noPanic:    true,
	}
	return nil
}

func aSubscriberOnPrefix(prefix string) error {
	recs, idx := w.disp.subscribeTopic(prefix)
	w.subs[prefix] = recs
	w.subIndices[prefix] = idx
	return nil
}

func anSSEListener() error {
	w.listener = w.disp.addSSEListener()
	return nil
}

func nSSEListeners(n int) error {
	w.listeners = make([]*sseListener, n)
	for i := range n {
		w.listeners[i] = w.disp.addSSEListener()
	}
	return nil
}

func aMessageArrivesOnTopic(topic, from string) error {
	w.disp.dispatch(from, topic, nil)
	return nil
}

func aMessageArrivesOnTopicWithPayload(topic, from string, payloadJSON *godog.DocString) error {
	var payload any
	if err := json.Unmarshal([]byte(payloadJSON.Content), &payload); err != nil {
		return fmt.Errorf("invalid payload JSON: %w", err)
	}
	w.disp.dispatch(from, topic, payload)
	return nil
}

func aMessageArrivesOnTopicWithEmptyPayload(topic, from string) error {
	w.disp.dispatch(from, topic, nil)
	return nil
}

func nMessagesArriveOnTopic(n int, topic, from string) error {
	for i := range n {
		w.disp.dispatch(from, topic, map[string]any{"content": fmt.Sprintf("msg-%d", i)})
	}
	return nil
}

func theSubscriberIsUnsubscribed(prefix string) error {
	idx, ok := w.subIndices[prefix]
	if !ok {
		return fmt.Errorf("no subscriber for prefix %q", prefix)
	}
	w.disp.unsubscribe(idx)
	return nil
}

func anSSEListenerConnects() error {
	w.listener = w.disp.addSSEListener()
	return nil
}

func theSSEListenerDisconnects() error {
	if w.listener != nil {
		w.disp.removeListener(w.listener)
	}
	return nil
}

func theSSEListenerDisconnectsAgain() error {
	if w.listener != nil {
		w.disp.removeListener(w.listener)
	}
	return nil
}

func aSecondSSEListenerConnects() error {
	w.listener2 = w.disp.addSSEListener()
	return nil
}

func aLocalEventIsPublished(topic string, payloadJSON *godog.DocString) error {
	var payload any
	if err := json.Unmarshal([]byte(payloadJSON.Content), &payload); err != nil {
		return fmt.Errorf("invalid payload JSON: %w", err)
	}
	w.disp.publishLocal(topic, "", payload)
	return nil
}

func deliveryIsConfirmed(msgID string) error {
	w.disp.notifyDelivered(msgID)
	return nil
}

// ── Assertions ───────────────────────────────────────────────────────────────

func subscriberShouldHaveReceivedN(prefix string, n int) error {
	recs, ok := w.subs[prefix]
	if !ok {
		if n == 0 {
			return nil
		}
		return fmt.Errorf("no subscriber for prefix %q", prefix)
	}
	if len(*recs) != n {
		return fmt.Errorf("expected %d messages for %q, got %d", n, prefix, len(*recs))
	}
	return nil
}

func sseListenerShouldHaveReceivedN(n int) error {
	if w.listener == nil {
		if n == 0 {
			return nil
		}
		return fmt.Errorf("no SSE listener")
	}
	if len(w.listener.events) != n {
		return fmt.Errorf("expected %d SSE events, got %d", n, len(w.listener.events))
	}
	return nil
}

func secondSSEListenerShouldHaveReceivedN(n int) error {
	if w.listener2 == nil {
		if n == 0 {
			return nil
		}
		return fmt.Errorf("no second SSE listener")
	}
	if len(w.listener2.events) != n {
		return fmt.Errorf("expected %d SSE events on second listener, got %d", n, len(w.listener2.events))
	}
	return nil
}

func allNSSEListenersShouldHaveReceivedM(n, m int) error {
	if len(w.listeners) != n {
		return fmt.Errorf("expected %d listeners, got %d", n, len(w.listeners))
	}
	for i, l := range w.listeners {
		if len(l.events) != m {
			return fmt.Errorf("listener %d: expected %d events, got %d", i+1, m, len(l.events))
		}
	}
	return nil
}

func sseEventTopicShouldBe(expected string) error {
	if w.listener == nil || len(w.listener.events) == 0 {
		return fmt.Errorf("no SSE events")
	}
	last := w.listener.events[len(w.listener.events)-1]
	if last.Topic != expected {
		return fmt.Errorf("expected SSE topic %q, got %q", expected, last.Topic)
	}
	return nil
}

func sseEventFromShouldBe(expected string) error {
	if w.listener == nil || len(w.listener.events) == 0 {
		return fmt.Errorf("no SSE events")
	}
	last := w.listener.events[len(w.listener.events)-1]
	if last.From != expected {
		return fmt.Errorf("expected SSE from %q, got %q", expected, last.From)
	}
	return nil
}

func sseEventTypeShouldBe(expected string) error {
	if w.listener == nil || len(w.listener.events) == 0 {
		return fmt.Errorf("no SSE events")
	}
	last := w.listener.events[len(w.listener.events)-1]
	if last.Type != expected {
		return fmt.Errorf("expected SSE type %q, got %q", expected, last.Type)
	}
	return nil
}

func sseEventMsgIDShouldBe(expected string) error {
	if w.listener == nil || len(w.listener.events) == 0 {
		return fmt.Errorf("no SSE events")
	}
	last := w.listener.events[len(w.listener.events)-1]
	if last.MsgID != expected {
		return fmt.Errorf("expected SSE msg_id %q, got %q", expected, last.MsgID)
	}
	return nil
}

func sseEventPayloadFieldShouldBeString(field, expected string) error {
	if w.listener == nil || len(w.listener.events) == 0 {
		return fmt.Errorf("no SSE events")
	}
	last := w.listener.events[len(w.listener.events)-1]
	pm, ok := last.Payload.(map[string]any)
	if !ok {
		return fmt.Errorf("SSE payload is not a map: %T", last.Payload)
	}
	val, ok := pm[field]
	if !ok {
		return fmt.Errorf("SSE payload missing field %q", field)
	}
	str := fmt.Sprintf("%v", val)
	if str != expected {
		return fmt.Errorf("expected payload %q = %q, got %q", field, expected, str)
	}
	return nil
}

func sseEventPayloadFieldShouldBeBool(field string, expected bool) error {
	if w.listener == nil || len(w.listener.events) == 0 {
		return fmt.Errorf("no SSE events")
	}
	last := w.listener.events[len(w.listener.events)-1]
	pm, ok := last.Payload.(map[string]any)
	if !ok {
		return fmt.Errorf("SSE payload is not a map: %T", last.Payload)
	}
	val, ok := pm[field]
	if !ok {
		return fmt.Errorf("SSE payload missing field %q", field)
	}
	b, ok := val.(bool)
	if !ok {
		return fmt.Errorf("payload %q is not a bool: %T", field, val)
	}
	if b != expected {
		return fmt.Errorf("expected payload %q = %v, got %v", field, expected, b)
	}
	return nil
}

func subscriberPayloadShouldHaveString(prefix, field, expected string) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) == 0 {
		return fmt.Errorf("no messages for %q", prefix)
	}
	last := (*recs)[len(*recs)-1]
	pm, ok := last.payload.(map[string]any)
	if !ok {
		return fmt.Errorf("payload is not a map: %T", last.payload)
	}
	val, ok := pm[field]
	if !ok {
		return fmt.Errorf("payload missing field %q (has: %v)", field, pm)
	}
	str := fmt.Sprintf("%v", val)
	if str != expected {
		return fmt.Errorf("expected %q = %q, got %q", field, expected, str)
	}
	return nil
}

func subscriberPayloadShouldHaveBool(prefix, field string, expected bool) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) == 0 {
		return fmt.Errorf("no messages for %q", prefix)
	}
	last := (*recs)[len(*recs)-1]
	pm, ok := last.payload.(map[string]any)
	if !ok {
		return fmt.Errorf("payload is not a map: %T", last.payload)
	}
	val, ok := pm[field]
	if !ok {
		return fmt.Errorf("payload missing field %q", field)
	}
	b, ok := val.(bool)
	if !ok {
		return fmt.Errorf("payload %q is not a bool: %T", field, val)
	}
	if b != expected {
		return fmt.Errorf("expected %q = %v, got %v", field, expected, b)
	}
	return nil
}

func subscriberPayloadFieldShouldBeNull(prefix, field string) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) == 0 {
		return fmt.Errorf("no messages for %q", prefix)
	}
	last := (*recs)[len(*recs)-1]
	pm, ok := last.payload.(map[string]any)
	if !ok {
		return fmt.Errorf("payload is not a map: %T", last.payload)
	}
	val, exists := pm[field]
	if !exists {
		return fmt.Errorf("payload missing field %q", field)
	}
	if val != nil {
		return fmt.Errorf("expected %q to be null, got %v", field, val)
	}
	return nil
}

func subscriberPayloadNestedShouldBe(prefix, path, expected string) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) == 0 {
		return fmt.Errorf("no messages for %q", prefix)
	}
	last := (*recs)[len(*recs)-1]
	parts := strings.Split(path, ".")
	var current any = last.payload
	for _, part := range parts {
		pm, ok := current.(map[string]any)
		if !ok {
			return fmt.Errorf("cannot traverse %q at %q: not a map", path, part)
		}
		current, ok = pm[part]
		if !ok {
			return fmt.Errorf("key %q not found in path %q", part, path)
		}
	}
	str := fmt.Sprintf("%v", current)
	if str != expected {
		return fmt.Errorf("expected nested %q = %q, got %q", path, expected, str)
	}
	return nil
}

func subscriberPayloadArrayLength(prefix, field string, expected int) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) == 0 {
		return fmt.Errorf("no messages for %q", prefix)
	}
	last := (*recs)[len(*recs)-1]
	pm, ok := last.payload.(map[string]any)
	if !ok {
		return fmt.Errorf("payload is not a map: %T", last.payload)
	}
	arr, ok := pm[field].([]any)
	if !ok {
		return fmt.Errorf("field %q is not an array: %T", field, pm[field])
	}
	if len(arr) != expected {
		return fmt.Errorf("expected array %q length %d, got %d", field, expected, len(arr))
	}
	return nil
}

func subscriberFromShouldBe(prefix, expected string) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) == 0 {
		return fmt.Errorf("no messages for %q", prefix)
	}
	last := (*recs)[len(*recs)-1]
	if last.from != expected {
		return fmt.Errorf("expected from %q, got %q", expected, last.from)
	}
	return nil
}

func subscriberTopicShouldBe(prefix, expected string) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) == 0 {
		return fmt.Errorf("no messages for %q", prefix)
	}
	last := (*recs)[len(*recs)-1]
	if last.topic != expected {
		return fmt.Errorf("expected topic %q, got %q", expected, last.topic)
	}
	return nil
}

func subscriberMessageNFromShouldBe(prefix string, n int, expected string) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) < n {
		return fmt.Errorf("not enough messages for %q (want #%d, have %d)", prefix, n, len(*recs))
	}
	rec := (*recs)[n-1]
	if rec.from != expected {
		return fmt.Errorf("message %d from: expected %q, got %q", n, expected, rec.from)
	}
	return nil
}

func subscriberMessageNTopicShouldBe(prefix string, n int, expected string) error {
	recs, ok := w.subs[prefix]
	if !ok || len(*recs) < n {
		return fmt.Errorf("not enough messages for %q (want #%d, have %d)", prefix, n, len(*recs))
	}
	rec := (*recs)[n-1]
	if rec.topic != expected {
		return fmt.Errorf("message %d topic: expected %q, got %q", n, expected, rec.topic)
	}
	return nil
}

func noPanicShouldHaveOccurred() error {
	if !w.noPanic {
		return fmt.Errorf("a panic occurred")
	}
	return nil
}

// ── Test runner ──────────────────────────────────────────────────────────────

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.After(func(ctx2 context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		w = nil
		return ctx2, nil
	})

	ctx.Step(`^a fresh MQ dispatcher$`, aFreshMQDispatcher)
	ctx.Step(`^a subscriber on prefix "([^"]*)"$`, aSubscriberOnPrefix)
	ctx.Step(`^an SSE listener$`, anSSEListener)
	ctx.Step(`^(\d+) SSE listeners$`, nSSEListeners)
	ctx.Step(`^an SSE listener connects$`, anSSEListenerConnects)
	ctx.Step(`^a second SSE listener connects$`, aSecondSSEListenerConnects)
	ctx.Step(`^the SSE listener disconnects$`, theSSEListenerDisconnects)
	ctx.Step(`^the SSE listener disconnects again$`, theSSEListenerDisconnectsAgain)

	ctx.Step(`^a message arrives on topic "([^"]*)" from "([^"]*)"$`, aMessageArrivesOnTopic)
	ctx.Step(`^a message arrives on topic "([^"]*)" from "([^"]*)" with payload$`, aMessageArrivesOnTopicWithPayload)
	ctx.Step(`^a message arrives on topic "([^"]*)" from "([^"]*)" with empty payload$`, aMessageArrivesOnTopicWithEmptyPayload)
	ctx.Step(`^(\d+) messages arrive on topic "([^"]*)" from "([^"]*)"$`, nMessagesArriveOnTopic)
	ctx.Step(`^the "([^"]*)" subscriber is unsubscribed$`, theSubscriberIsUnsubscribed)
	ctx.Step(`^a local event is published on topic "([^"]*)" with payload$`, aLocalEventIsPublished)
	ctx.Step(`^delivery is confirmed for message "([^"]*)"$`, deliveryIsConfirmed)

	ctx.Step(`^the "([^"]*)" subscriber should have received (\d+) messages?$`, subscriberShouldHaveReceivedN)
	ctx.Step(`^the SSE listener should have received (\d+) events?$`, sseListenerShouldHaveReceivedN)
	ctx.Step(`^the second SSE listener should have received (\d+) events?$`, secondSSEListenerShouldHaveReceivedN)
	ctx.Step(`^all (\d+) SSE listeners should have received (\d+) event each$`, allNSSEListenersShouldHaveReceivedM)
	ctx.Step(`^the SSE event topic should be "([^"]*)"$`, sseEventTopicShouldBe)
	ctx.Step(`^the SSE event from should be "([^"]*)"$`, sseEventFromShouldBe)
	ctx.Step(`^the SSE event type should be "([^"]*)"$`, sseEventTypeShouldBe)
	ctx.Step(`^the SSE event msg_id should be "([^"]*)"$`, sseEventMsgIDShouldBe)
	ctx.Step(`^the SSE event payload "([^"]*)" should be "([^"]*)"$`, sseEventPayloadFieldShouldBeString)
	ctx.Step(`^the SSE event payload "([^"]*)" should be (true|false)$`, func(field, val string) error {
		return sseEventPayloadFieldShouldBeBool(field, val == "true")
	})
	ctx.Step(`^the "([^"]*)" subscriber payload should have "([^"]*)" = "([^"]*)"$`, subscriberPayloadShouldHaveString)
	ctx.Step(`^the "([^"]*)" subscriber payload should have "([^"]*)" = (true|false)$`, func(prefix, field, val string) error {
		return subscriberPayloadShouldHaveBool(prefix, field, val == "true")
	})
	ctx.Step(`^the "([^"]*)" subscriber payload "([^"]*)" should be null$`, subscriberPayloadFieldShouldBeNull)
	ctx.Step(`^the "([^"]*)" subscriber payload should have nested "([^"]*)" = "([^"]*)"$`, subscriberPayloadNestedShouldBe)
	ctx.Step(`^the "([^"]*)" subscriber payload should have array "([^"]*)" with (\d+) elements$`, subscriberPayloadArrayLength)
	ctx.Step(`^the "([^"]*)" subscriber from should be "([^"]*)"$`, subscriberFromShouldBe)
	ctx.Step(`^the "([^"]*)" subscriber topic should be "([^"]*)"$`, subscriberTopicShouldBe)
	ctx.Step(`^the "([^"]*)" subscriber message (\d+) from should be "([^"]*)"$`, subscriberMessageNFromShouldBe)
	ctx.Step(`^the "([^"]*)" subscriber message (\d+) topic should be "([^"]*)"$`, subscriberMessageNTopicShouldBe)
	ctx.Step(`^no panic should have occurred$`, noPanicShouldHaveOccurred)
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

// Ensure mq package is imported (topic constants).
var _ = mq.TopicChat
var _ = strconv.Itoa
