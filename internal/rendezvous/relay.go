package rendezvous

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	ymux "github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	pbv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/pb"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	yamux "github.com/libp2p/go-yamux/v4"
	ma "github.com/multiformats/go-multiaddr"
)

// RelayInfo describes a circuit relay v2 host that peers can use for NAT traversal.
type RelayInfo struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs"`

	// Timing values pushed from the server config.
	CleanupDelaySec    int `json:"cleanup_delay_sec"`
	PollDeadlineSec    int `json:"poll_deadline_sec"`
	ConnectTimeoutSec  int `json:"connect_timeout_sec"`
	RefreshIntervalSec int `json:"refresh_interval_sec"`
	RecoveryGraceSec   int `json:"recovery_grace_sec"`
}

// relayTracer logs circuit relay events to the relay log via a callback.
type relayTracer struct {
	logFn          func(string)
	openCircuits   atomic.Int32
	bytesRelayed   atomic.Int64
}

func (t *relayTracer) RelayStatus(enabled bool) {
	if enabled {
		t.logFn("relay service enabled")
	} else {
		t.logFn("relay service disabled")
	}
}

func (t *relayTracer) ConnectionOpened() {
	n := t.openCircuits.Add(1)
	t.logFn(fmt.Sprintf("CIRCUIT opened (active: %d)", n))
}

func (t *relayTracer) ConnectionClosed(d time.Duration) {
	n := t.openCircuits.Add(-1)
	t.logFn(fmt.Sprintf("CIRCUIT closed after %s (active: %d)", d.Truncate(time.Second), n))
}

func (t *relayTracer) ConnectionRequestHandled(status pbv2.Status) {
	if status != pbv2.Status_OK {
		t.logFn(fmt.Sprintf("CIRCUIT request: %s", statusString(status)))
	}
}

func (t *relayTracer) ReservationAllowed(isRenewal bool) {
	if isRenewal {
		t.logFn("reservation renewed")
	} else {
		t.logFn("reservation created")
	}
}

func (t *relayTracer) ReservationClosed(cnt int) {
	if cnt > 0 {
		t.logFn(fmt.Sprintf("reservation closed (%d expired)", cnt))
	}
}

func (t *relayTracer) ReservationRequestHandled(status pbv2.Status) {
	if status != pbv2.Status_OK {
		t.logFn(fmt.Sprintf("reservation request: %s", statusString(status)))
	}
}

func (t *relayTracer) BytesTransferred(cnt int) {
	t.bytesRelayed.Add(int64(cnt))
}

func statusString(s pbv2.Status) string {
	switch s {
	case pbv2.Status_OK:
		return "OK"
	case pbv2.Status_RESERVATION_REFUSED:
		return "RESERVATION_REFUSED"
	case pbv2.Status_RESOURCE_LIMIT_EXCEEDED:
		return "RESOURCE_LIMIT_EXCEEDED"
	case pbv2.Status_PERMISSION_DENIED:
		return "PERMISSION_DENIED"
	case pbv2.Status_NO_RESERVATION:
		return "NO_RESERVATION"
	case pbv2.Status_CONNECTION_FAILED:
		return "CONNECTION_FAILED"
	case pbv2.Status_MALFORMED_MESSAGE:
		return "MALFORMED_MESSAGE"
	default:
		return fmt.Sprintf("status_%d", s)
	}
}

// StartRelay creates a libp2p host that acts as a circuit relay v2 server.
// externalURL, if set, is used to derive the public IP so WAN peers get a
// reachable address (e.g. /ip4/<public>/tcp/<port>/p2p/<id>).
// logFn is called for circuit events (nil = log.Printf).
func StartRelay(port int, wsPort int, keyFile string, externalURL string, logFn func(string)) (host.Host, *RelayInfo, error) {
	priv, err := loadOrCreateRelayKey(keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("relay key: %w", err)
	}

	ymuxCfg := yamux.DefaultConfig()
	ymuxCfg.KeepAliveInterval = RelayYamuxKeepAlive
	ymuxCfg.LogOutput = io.Discard

	listenAddrs := []string{fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)}
	if wsPort > 0 {
		listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ws", wsPort))
	}

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(listenAddrs...),
		libp2p.DisableRelay(),
		libp2p.Muxer(ymux.ID, (*ymux.Transport)(ymuxCfg)),
	}

	if externalURL != "" {
		extAddrs := buildExternalAddrs(externalURL, port, wsPort)
		if len(extAddrs) > 0 {
			opts = append(opts, libp2p.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
				return append(addrs, extAddrs...)
			}))
		}
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("relay host: %w", err)
	}

	// Start the relay service directly instead of using EnableRelayService(),
	// which defers startup until AutoNAT confirms public reachability.
	// Since this is a dedicated relay server with port forwarding, we know
	// it's publicly reachable — no need to wait for AutoNAT.
	//
	// Default limits are too restrictive for a private relay (Duration: 2min,
	// Data: 128KB per relayed connection). GossipSub heartbeats exhaust
	// these quickly, killing the data path while TCP stays alive.
	if logFn == nil {
		logFn = func(msg string) { log.Print(msg) }
	}
	tracer := &relayTracer{logFn: logFn}
	if _, err := relayv2.New(h, relayv2.WithResources(relayv2.Resources{
		Limit: &relayv2.RelayLimit{
			Duration: RelayDuration,
			Data:     1 << 24, // 16 MB
		},
		ReservationTTL:         RelayReservationTTL,
		MaxReservations:        RelayMaxReservations,
		MaxCircuits:            RelayMaxCircuits,
		BufferSize:             4096,
		MaxReservationsPerPeer: RelayMaxPerPeer,
		MaxReservationsPerIP:   RelayMaxPerIP,
		MaxReservationsPerASN:  RelayMaxPerASN,
	}), relayv2.WithMetricsTracer(tracer)); err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("relay service: %w", err)
	}

	info := &RelayInfo{
		PeerID: h.ID().String(),
	}

	// Collect listen addresses.
	for _, a := range h.Addrs() {
		info.Addrs = append(info.Addrs, a.String())
	}

	if externalURL != "" {
		if wsPort > 0 {
			if wssAddr := buildWSSAddr(externalURL, h.ID().String()); wssAddr != "" {
				info.Addrs = append(info.Addrs, wssAddr)
			}
		} else {
			if pubAddr := buildPublicAddr(externalURL, port, h.ID().String()); pubAddr != "" {
				info.Addrs = append(info.Addrs, pubAddr)
			}
		}
	}

	log.Printf("relay: listening on port %d (ws %d), peer ID %s (%d addrs)", port, wsPort, info.PeerID, len(info.Addrs))
	return h, info, nil
}

// buildPublicAddr resolves the hostname from externalURL and returns a
// multiaddr string like /ip4/<ip>/tcp/<port>/p2p/<id>.
func buildPublicAddr(externalURL string, port int, peerID string) string {
	u, err := url.Parse(externalURL)
	if err != nil {
		return ""
	}
	hostname := u.Hostname()
	if hostname == "" {
		return ""
	}

	// Resolve to IP if it's a domain name.
	ip := net.ParseIP(hostname)
	if ip == nil {
		ips, err := net.LookupIP(hostname)
		if err != nil || len(ips) == 0 {
			log.Printf("relay: could not resolve %s: %v", hostname, err)
			return ""
		}
		// Prefer IPv4.
		for _, candidate := range ips {
			if candidate.To4() != nil {
				ip = candidate
				break
			}
		}
		if ip == nil {
			ip = ips[0]
		}
	}

	if ip.To4() != nil {
		return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ip.String(), port, peerID)
	}
	return fmt.Sprintf("/ip6/%s/tcp/%d/p2p/%s", ip.String(), port, peerID)
}

func buildExternalAddrs(externalURL string, tcpPort, wsPort int) []ma.Multiaddr {
	var addrs []ma.Multiaddr
	u, err := url.Parse(externalURL)
	if err != nil {
		return nil
	}
	hostname := u.Hostname()
	if hostname == "" {
		return nil
	}

	ip := net.ParseIP(hostname)
	if ip == nil {
		ips, err := net.LookupIP(hostname)
		if err != nil || len(ips) == 0 {
			return nil
		}
		for _, candidate := range ips {
			if candidate.To4() != nil {
				ip = candidate
				break
			}
		}
		if ip == nil {
			ip = ips[0]
		}
	}

	if tcpAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ip.String(), tcpPort)); err == nil {
		addrs = append(addrs, tcpAddr)
	}
	if wsPort > 0 {
		if wssAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/443/tls/sni/%s/ws", ip.String(), hostname)); err == nil {
			addrs = append(addrs, wssAddr)
		}
	}
	return addrs
}

func buildWSSAddr(externalURL string, peerID string) string {
	u, err := url.Parse(externalURL)
	if err != nil {
		return ""
	}
	hostname := u.Hostname()
	if hostname == "" {
		return ""
	}

	ip := net.ParseIP(hostname)
	if ip == nil {
		ips, err := net.LookupIP(hostname)
		if err != nil || len(ips) == 0 {
			return ""
		}
		for _, candidate := range ips {
			if candidate.To4() != nil {
				ip = candidate
				break
			}
		}
		if ip == nil {
			ip = ips[0]
		}
	}
	return fmt.Sprintf("/ip4/%s/tcp/443/tls/sni/%s/ws/p2p/%s", ip.String(), hostname, peerID)
}

// loadOrCreateRelayKey loads an Ed25519 key from disk, or creates one.
func loadOrCreateRelayKey(keyFile string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(keyFile)
	if err == nil {
		priv, err := crypto.UnmarshalPrivateKey(data)
		if err == nil {
			return priv, nil
		}
		log.Printf("WARNING: corrupt relay key at %s: %v (generating new key)", keyFile, err)
	}

	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, err
	}

	raw, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal relay key: %w", err)
	}

	if dir := filepath.Dir(keyFile); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create relay key directory: %w", err)
		}
	}

	if err := os.WriteFile(keyFile, raw, 0600); err != nil {
		return nil, fmt.Errorf("save relay key: %w", err)
	}

	log.Printf("relay: generated new identity key: %s", keyFile)
	return priv, nil
}

// handleRelayInfo writes the RelayInfo as JSON. Returns 404 if info is nil.
func handleRelayInfo(w http.ResponseWriter, r *http.Request, info *RelayInfo) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if info == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(info)
}
