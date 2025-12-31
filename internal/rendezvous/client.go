// internal/rendezvous/client.go
package rendezvous

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"goop/internal/proto"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		HTTP: &http.Client{
			Timeout: 10 * time.Second, // for publish requests
		},
	}
}

func (c *Client) Publish(ctx context.Context, pm proto.PresenceMsg) error {
	if c.BaseURL == "" {
		return nil
	}

	if pm.TS == 0 {
		pm.TS = proto.NowMillis()
	}

	b, _ := json.Marshal(pm)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/publish", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("publish status %s", resp.Status)
	}
	return nil
}

// SubscribeEvents connects to /events and calls onMsg for each "data: <json>" message.
// It reconnects automatically with a small backoff until ctx is cancelled.
func (c *Client) SubscribeEvents(ctx context.Context, onMsg func(proto.PresenceMsg)) {
	if c.BaseURL == "" {
		return
	}

	backoff := 250 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.subscribeOnce(ctx, onMsg)
		_ = err // optional: log outside, in caller

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) subscribeOnce(ctx context.Context, onMsg func(proto.PresenceMsg)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/events", nil)
	if err != nil {
		return err
	}

	// No client timeout for SSE; use ctx for cancellation.
	httpNoTimeout := &http.Client{}
	resp, err := httpNoTimeout.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("events status %s", resp.Status)
	}

	sc := bufio.NewScanner(resp.Body)
	// "data: <json>" lines; blank line separates events; ":" comments possible.
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		var pm proto.PresenceMsg
		if err := json.Unmarshal([]byte(payload), &pm); err != nil {
			continue
		}

		if pm.Type == "" || pm.PeerID == "" {
			continue
		}

		if onMsg != nil {
			onMsg(pm)
		}
	}
	return sc.Err()
}
