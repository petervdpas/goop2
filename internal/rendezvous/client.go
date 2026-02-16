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

	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/util"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = util.NormalizeURL(baseURL)
	return &Client{
		BaseURL: baseURL,
		HTTP: &http.Client{
			Timeout: 10 * time.Second, // for publish requests
		},
	}
}

// getJSON performs a GET request, drains the response body, and decodes JSON
// into v. Returns (true, nil) on 2xx. Returns (false, nil) if the server
// returns 404 or 502 (endpoint not available). Returns (false, err) on other
// non-2xx status or transport/decode errors.
func (c *Client) getJSON(ctx context.Context, url string, v any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadGateway {
		return false, nil
	}
	if resp.StatusCode/100 != 2 {
		return false, fmt.Errorf("GET %s: status %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return false, err
	}
	return true, nil
}

// FetchRelayInfo fetches relay info from the rendezvous server.
// Returns (nil, nil) if the server has no relay enabled.
func (c *Client) FetchRelayInfo(ctx context.Context) (*RelayInfo, error) {
	if c.BaseURL == "" {
		return nil, nil
	}
	var info RelayInfo
	found, err := c.getJSON(ctx, c.BaseURL+"/relay", &info)
	if !found || err != nil {
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
// peerID is sent so the server can gate access based on registration status.
// Returns nil (not an error) if the server has no template store.
func (c *Client) ListTemplates(ctx context.Context, peerID string) ([]StoreMeta, error) {
	if c.BaseURL == "" {
		return nil, nil
	}
	u := c.BaseURL + "/api/templates"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if peerID != "" {
		req.Header.Set("X-Goop-Peer-ID", peerID)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadGateway {
		return nil, nil
	}
	if resp.StatusCode == http.StatusForbidden {
		// Server says peer is not allowed — read message
		body, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = "access denied"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("GET %s: status %s", u, resp.Status)
	}
	var out []StoreMeta
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// BalanceResult holds the credit balance info for a peer.
type BalanceResult struct {
	Active  bool `json:"credits_active"`
	Balance int  `json:"balance"`
}

// FetchBalance fetches the credit balance for a peer from the credits service
// via the rendezvous server's /api/credits/store-data proxy.
// Returns a zero BalanceResult (Active=false) if credits are not configured.
func (c *Client) FetchBalance(ctx context.Context, peerID string) (BalanceResult, error) {
	if c.BaseURL == "" {
		return BalanceResult{}, nil
	}
	reqURL := c.BaseURL + "/api/credits/store-data"
	if peerID != "" {
		reqURL += "?peer_id=" + peerID
	}
	var data struct {
		CreditsActive bool `json:"credits_active"`
		Balance       int  `json:"balance"`
	}
	found, err := c.getJSON(ctx, reqURL, &data)
	if !found || err != nil {
		return BalanceResult{}, err
	}
	return BalanceResult{Active: data.CreditsActive, Balance: data.Balance}, nil
}

// FetchOwnedTemplates fetches the list of template dirs owned by the peer.
// Returns nil if credits are not configured or the peer has no owned templates.
func (c *Client) FetchOwnedTemplates(ctx context.Context, peerID string) (map[string]bool, error) {
	if c.BaseURL == "" {
		return nil, nil
	}
	reqURL := c.BaseURL + "/api/credits/store-data"
	if peerID != "" {
		reqURL += "?peer_id=" + peerID
	}
	var data struct {
		OwnedTemplates []string `json:"owned_templates"`
	}
	found, err := c.getJSON(ctx, reqURL, &data)
	if !found || err != nil || len(data.OwnedTemplates) == 0 {
		return nil, err
	}
	owned := make(map[string]bool, len(data.OwnedTemplates))
	for _, dir := range data.OwnedTemplates {
		owned[dir] = true
	}
	return owned, nil
}

// FetchPrices fetches template prices from the templates service via the
// rendezvous server's /api/templates/prices proxy.
// Returns nil (not an error) if the endpoint is unavailable.
func (c *Client) FetchPrices(ctx context.Context) (map[string]int, error) {
	if c.BaseURL == "" {
		return nil, nil
	}
	var out map[string]int
	found, err := c.getJSON(ctx, c.BaseURL+"/api/templates/prices", &out)
	if !found || err != nil {
		return nil, err
	}
	return out, nil
}

// SpendResult holds the response from a credit spend call.
type SpendResult struct {
	Balance int  `json:"balance"`
	Owned   bool `json:"owned"`
}

// SpendCredits calls POST /api/credits/spend to deduct credits and grant
// template ownership. peerID is sent as X-Goop-Peer-ID for email resolution.
// Returns the spend result (new balance + ownership) on success.
// Returns an error on insufficient credits or service failure.
func (c *Client) SpendCredits(ctx context.Context, templateDir, peerID string) (*SpendResult, error) {
	if c.BaseURL == "" {
		return nil, nil
	}

	body, _ := json.Marshal(map[string]string{"template": templateDir})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/credits/spend", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if peerID != "" {
		req.Header.Set("X-Goop-Peer-ID", peerID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("credits spend: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusPaymentRequired {
		return nil, fmt.Errorf("Template could not be applied, insufficient funding")
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("credits spend: status %s", resp.Status)
	}

	var result SpendResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Spend succeeded (2xx) but couldn't parse response — treat as success
		return &SpendResult{Owned: true}, nil
	}
	return &result, nil
}

// DownloadTemplateBundle fetches the tar.gz bundle for a store template.
// peerID is sent as X-Goop-Peer-ID so the server can verify registration.
// Caller must close the returned ReadCloser.
func (c *Client) DownloadTemplateBundle(ctx context.Context, dir, peerID string) (io.ReadCloser, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("no base url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/templates/"+dir+"/bundle", nil)
	if err != nil {
		return nil, err
	}
	if peerID != "" {
		req.Header.Set("X-Goop-Peer-ID", peerID)
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

// PulsePeer asks the rendezvous server to tell a target peer to refresh its
// relay reservation. This is called by the requesting peer when it can't
// reach the target through the relay.
func (c *Client) PulsePeer(ctx context.Context, peerID string) error {
	if c.BaseURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/pulse?peer="+peerID, nil)
	if err != nil {
		return err
	}
	// Use a client without the default 10s timeout — the pulse operation
	// triggers relay refresh on the target (up to 23s). The ctx controls
	// the actual deadline.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("pulse: status %s", resp.Status)
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
