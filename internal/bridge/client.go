package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/state"
)

// Client connects a thin-client peer to the bridge service.
type Client struct {
	bridgeURL  string
	bridgeHost string
	email      string
	token      string
	peerID     string
	label      string
	publicKey  string
	encSupport bool
	peers      *state.PeerTable
	httpClient *http.Client

	dnsMu      sync.RWMutex
	dnsIP      string
	dnsExpires time.Time
}

// New creates a bridge client.
func New(bridgeURL, email, token, peerID, label, publicKey string, encSupport bool, peers *state.PeerTable) *Client {
	bridgeURL = strings.TrimRight(bridgeURL, "/")

	var bridgeHost string
	if u, err := url.Parse(bridgeURL); err == nil {
		bridgeHost = u.Hostname()
	}

	c := &Client{
		bridgeURL:  bridgeURL,
		bridgeHost: bridgeHost,
		email:      email,
		token:      token,
		peerID:     peerID,
		label:      label,
		publicKey:  publicKey,
		encSupport: encSupport,
		peers:      peers,
	}

	c.httpClient = &http.Client{
		Timeout:   HTTPClientTimeout,
		Transport: &http.Transport{DialContext: c.dialContext},
	}
	return c
}

func (c *Client) resolveHost(ctx context.Context) (string, error) {
	if c.bridgeHost == "" || net.ParseIP(c.bridgeHost) != nil {
		return c.bridgeHost, nil
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

	ips, err := net.DefaultResolver.LookupHost(resolveCtx, c.bridgeHost)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", c.bridgeHost, err)
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

	log.Printf("bridge: resolved %s → %s", c.bridgeHost, chosen)
	return chosen, nil
}

func (c *Client) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	if host != c.bridgeHost {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	ip, err := c.resolveHost(context.Background())
	if err != nil {
		return nil, err
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip, port))
	if err != nil {
		c.dnsMu.Lock()
		c.dnsIP = ""
		c.dnsExpires = time.Time{}
		c.dnsMu.Unlock()
		return nil, err
	}
	return conn, nil
}

// Register registers this peer as a VPeer on the bridge service.
func (c *Client) Register(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"peer_id":              c.peerID,
		"label":                c.label,
		"email":                c.email,
		"platform":             "desktop",
		"public_key":           c.publicKey,
		"encryption_supported": c.encSupport,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.bridgeURL+"/api/bridge/peers", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goop-Email", c.email)
	req.Header.Set("X-Bridge-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bridge register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("bridge register: status %d", resp.StatusCode)
	}

	var result struct {
		SessionID string `json:"session_id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	log.Printf("bridge: registered as VPeer %s (session %s)", c.peerID, result.SessionID)
	return result.SessionID, nil
}

// Connect opens the WebSocket tunnel and processes events until ctx is cancelled.
// Reconnects automatically on disconnect.
func (c *Client) Connect(ctx context.Context, onPresence func(data json.RawMessage)) {
	backoff := WSReconnectBackoff
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if _, err := c.Register(ctx); err != nil {
			log.Printf("bridge: register failed: %v", err)
		}

		err := c.connectOnce(ctx, onPresence)
		if err != nil {
			log.Printf("bridge ws: %v (reconnecting)", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < WSReconnectMaxBackoff {
			backoff *= 2
		}
	}
}

func (c *Client) connectOnce(ctx context.Context, onPresence func(data json.RawMessage)) error {
	wsURL := c.bridgeURL
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/bridge/ws/" + c.peerID

	dialer := websocket.Dialer{HandshakeTimeout: WSHandshakeTimeout, NetDialContext: c.dialContext}
	header := http.Header{}
	header.Set("X-Goop-Email", c.email)
	header.Set("X-Bridge-Token", c.token)

	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("bridge ws dial: %w", err)
	}
	defer conn.Close()

	log.Printf("bridge ws: connected to %s", wsURL)

	conn.SetReadDeadline(time.Now().Add(WSReadDeadline))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(WSReadDeadline))
		return nil
	})

	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(WSPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
				msg, _ := json.Marshal(map[string]string{"type": "ping"})
				conn.WriteMessage(websocket.TextMessage, msg)
			case <-pingDone:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	defer close(pingDone)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("bridge ws read: %w", err)
		}
		conn.SetReadDeadline(time.Now().Add(WSReadDeadline))

		var evt struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(message, &evt) != nil {
			continue
		}

		switch evt.Type {
		case "presence":
			if onPresence != nil {
				onPresence(evt.Data)
			}
		}
	}
}
