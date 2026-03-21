package rendezvous

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/util"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client

	rvHost string // hostname extracted from BaseURL (for DNS cache matching)

	dnsMu      sync.RWMutex
	dnsIP      string    // cached resolved IP for rvHost
	dnsExpires time.Time // when the cache entry expires

	// WebSocket state (set by ConnectWebSocket)
	wsMu   sync.Mutex
	wsConn *websocket.Conn
	wsSend chan []byte // buffered send channel for write pump
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = util.NormalizeURL(baseURL)

	var rvHost string
	if u, err := url.Parse(baseURL); err == nil {
		rvHost = u.Hostname()
	}

	c := &Client{
		BaseURL: baseURL,
		rvHost:  rvHost,
	}

	c.HTTP = &http.Client{
		Timeout: HTTPClientTimeout,
		Transport: &http.Transport{
			DialContext: c.dialContext,
		},
	}
	return c
}

// WarmDNS resolves the rendezvous hostname and caches the result.
// Call before making HTTP/WS requests to avoid DNS eating into request timeouts.
func (c *Client) WarmDNS(ctx context.Context) {
	if _, err := c.resolveHost(ctx); err != nil {
		log.Printf("rendezvous: %s unreachable (DNS failed: %v)", c.BaseURL, err)
	}
}

// DNSReady returns true if the hostname has been resolved successfully.
func (c *Client) DNSReady() bool {
	if c.rvHost == "" || net.ParseIP(c.rvHost) != nil {
		return true
	}
	c.dnsMu.RLock()
	defer c.dnsMu.RUnlock()
	return c.dnsIP != ""
}

func (c *Client) resolveHost(ctx context.Context) (string, error) {
	if c.rvHost == "" || net.ParseIP(c.rvHost) != nil {
		return c.rvHost, nil
	}

	c.dnsMu.RLock()
	if c.dnsIP != "" && time.Now().Before(c.dnsExpires) {
		ip := c.dnsIP
		c.dnsMu.RUnlock()
		return ip, nil
	}
	c.dnsMu.RUnlock()

	resolveCtx, cancel := context.WithTimeout(ctx, DNSResolveTimeout)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(resolveCtx, c.rvHost)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", c.rvHost, err)
	}

	chosen := ips[0]
	for _, ip := range ips {
		if net.ParseIP(ip) != nil && net.ParseIP(ip).To4() != nil {
			chosen = ip
			break
		}
	}

	c.dnsMu.Lock()
	c.dnsIP = chosen
	c.dnsExpires = time.Now().Add(DNSCacheTTL)
	c.dnsMu.Unlock()

	log.Printf("rendezvous: resolved %s → %s", c.rvHost, chosen)
	return chosen, nil
}

func (c *Client) clearDNSCache() {
	c.dnsMu.Lock()
	c.dnsIP = ""
	c.dnsExpires = time.Time{}
	c.dnsMu.Unlock()
}

func (c *Client) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	if host != c.rvHost {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	ip, err := c.resolveHost(context.Background())
	if err != nil {
		return nil, err
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip, port))
	if err != nil {
		c.clearDNSCache()
		return nil, err
	}
	return conn, nil
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

// RegisterEncryptionKey registers the peer's NaCl public key with the
// encryption service via the rendezvous proxy.
func (c *Client) RegisterEncryptionKey(ctx context.Context, peerID, publicKey string) error {
	if c.BaseURL == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"peer_id": peerID, "public_key": publicKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/encryption/keys", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goop-PeerID", peerID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("register encryption key: status %s", resp.Status)
	}
	return nil
}

// FetchPeerKey fetches a remote peer's NaCl public key from the encryption
// service via the rendezvous proxy. Returns empty string + nil if not found.
func (c *Client) FetchPeerKey(ctx context.Context, peerID string) (string, error) {
	if c.BaseURL == "" {
		return "", nil
	}
	var result struct {
		PublicKey string `json:"public_key"`
	}
	found, err := c.getJSON(ctx, c.BaseURL+"/api/encryption/keys/"+peerID, &result)
	if !found || err != nil {
		return "", err
	}
	return result.PublicKey, nil
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

	backoff := WSBackoff
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
		if backoff < SSEReconnectBackoff {
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

// ConnectWebSocket tries to establish a WebSocket connection to the rendezvous
// server. If the server doesn't support WebSocket (older rendezvous, 404, upgrade
// rejected), it falls back to SSE via SubscribeEvents, then retries WS
// periodically in case the server gets upgraded.
func (c *Client) ConnectWebSocket(ctx context.Context, peerID string, onMsg func(proto.PresenceMsg)) {
	if c.BaseURL == "" {
		return
	}

	backoff := WSBackoff
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.connectWSOnce(ctx, peerID, onMsg)

		c.wsMu.Lock()
		c.wsConn = nil
		c.wsSend = nil
		c.wsMu.Unlock()

		if err != nil {
			if isWSTooEarly(err) {
				log.Printf("rendezvous ws: server says publish first (425), retrying in %v", WSBackoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(WSBackoff):
				}
				continue
			}
			if isWSUnsupported(err) {
				log.Printf("rendezvous: WS unavailable at %s, using SSE (probing WS in %v)", c.BaseURL, WSProbeFirstInterval)
				sseCtx, sseCancel := context.WithCancel(ctx)
				go c.SubscribeEvents(sseCtx, onMsg)

				probeWait := WSProbeFirstInterval
				for {
					select {
					case <-ctx.Done():
						sseCancel()
						return
					case <-time.After(probeWait):
					}
					if c.probeWS(ctx) {
						log.Printf("rendezvous: WS now available at %s, switching from SSE", c.BaseURL)
						sseCancel()
						backoff = WSBackoff
						break
					}
					probeWait = WSProbeNextInterval
				}
				continue
			}
			log.Printf("rendezvous ws: %v (reconnecting)", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < WSReconnectBackoff {
			backoff *= 2
		}
	}
}

// probeWS checks if the server supports WebSocket without disrupting any
// existing connection. Opens a WS, immediately closes it, returns success.
func (c *Client) probeWS(ctx context.Context) bool {
	wsURL := c.wsProbeURL()
	probeCtx, cancel := context.WithTimeout(ctx, WSProbeTimeout)
	defer cancel()

	conn, _, err := (&websocket.Dialer{HandshakeTimeout: WSProbeTimeout, NetDialContext: c.dialContext}).DialContext(probeCtx, wsURL, nil)
	if err != nil {
		return false
	}
	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "probe"))
	conn.Close()
	return true
}

// isWSUnsupported returns true if the error indicates the server does not
// support WebSocket (as opposed to a transient network failure).
func isWSUnsupported(err error) bool {
	s := err.Error()
	return strings.Contains(s, "bad handshake") ||
		strings.Contains(s, "404") ||
		strings.Contains(s, "403") ||
		strings.Contains(s, "501")
}

// isWSTooEarly returns true if the server rejected the WS because the peer
// hasn't published yet (425 Too Early). The client should retry shortly.
func isWSTooEarly(err error) bool {
	return strings.Contains(err.Error(), "425")
}

func (c *Client) wsBase() string {
	u := c.BaseURL
	if after, ok := strings.CutPrefix(u, "http://"); ok {
		host := after
		if idx := strings.Index(host, "/"); idx != -1 {
			host = host[:idx]
		}
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			u = "https://" + after
		}
	}
	u = strings.Replace(u, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	return u + "/ws"
}

func (c *Client) wsURL(peerID string) string {
	return c.wsBase() + "?peer_id=" + peerID
}

func (c *Client) wsProbeURL() string {
	return c.wsBase() + "?probe=1"
}

func (c *Client) connectWSOnce(ctx context.Context, peerID string, onMsg func(proto.PresenceMsg)) error {
	wsURL := c.wsURL(peerID)

	dialer := websocket.Dialer{
		HandshakeTimeout: WSHandshakeTimeout,
		NetDialContext:   c.dialContext,
	}

	conn, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
			resp.Body.Close()
		}
		return fmt.Errorf("ws dial %s (status %d): %w", wsURL, status, err)
	}

	log.Printf("rendezvous ws: connected to %s", c.BaseURL)

	sendCh := make(chan []byte, 64)

	c.wsMu.Lock()
	c.wsConn = conn
	c.wsSend = sendCh
	c.wsMu.Unlock()

	// Write pump
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case msg, ok := <-sendCh:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ctx.Done():
				conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
		}
	}()

	// Read pump
	conn.SetReadDeadline(time.Now().Add(WSReadDeadline))
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(WSReadDeadline))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(WSWriteDeadline))
	})
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(WSReadDeadline))
		return nil
	})

	defer func() {
		close(sendCh)
		conn.Close()
		<-done // wait for write pump to exit
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("ws read: %w", err)
		}

		conn.SetReadDeadline(time.Now().Add(WSReadDeadline))

		var pm proto.PresenceMsg
		if err := json.Unmarshal(message, &pm); err != nil {
			continue
		}
		if pm.Type == "" || pm.PeerID == "" {
			continue
		}
		if onMsg != nil {
			onMsg(pm)
		}
	}
}

// PublishWS sends a presence message via the active WebSocket connection.
// Returns false if no WebSocket is connected (caller should fall back to POST).
func (c *Client) PublishWS(pm proto.PresenceMsg) bool {
	c.wsMu.Lock()
	sendCh := c.wsSend
	c.wsMu.Unlock()

	if sendCh == nil {
		return false
	}

	b, err := json.Marshal(pm)
	if err != nil {
		return false
	}

	select {
	case sendCh <- b:
		return true
	default:
		return false // buffer full
	}
}

// IsWebSocketConnected returns true if this client has an active WebSocket connection.
func (c *Client) IsWebSocketConnected() bool {
	c.wsMu.Lock()
	defer c.wsMu.Unlock()
	return c.wsConn != nil
}
