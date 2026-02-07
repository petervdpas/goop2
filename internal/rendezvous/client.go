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

// FetchRelayInfo fetches relay info from the rendezvous server.
// Returns (nil, nil) if the server has no relay enabled (404).
func (c *Client) FetchRelayInfo(ctx context.Context) (*RelayInfo, error) {
	if c.BaseURL == "" {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/relay", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("fetch relay info: status %s", resp.Status)
	}

	var info RelayInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
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

// ListTemplates fetches the template store listing from the rendezvous server.
// Returns nil (not an error) if the server has no template store (404).
func (c *Client) ListTemplates(ctx context.Context) ([]StoreMeta, error) {
	if c.BaseURL == "" {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/templates", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("list templates: status %s", resp.Status)
	}

	var out []StoreMeta
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// DownloadTemplateBundle fetches the tar.gz bundle for a store template.
// Caller must close the returned ReadCloser.
func (c *Client) DownloadTemplateBundle(ctx context.Context, dir string) (io.ReadCloser, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("no base url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/templates/"+dir+"/bundle", nil)
	if err != nil {
		return nil, err
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode/100 != 2 {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("download bundle: status %s", resp.Status)
	}

	return resp.Body, nil
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
