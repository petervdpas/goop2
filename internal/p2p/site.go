
package p2p

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/proto"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
)

func (n *Node) EnableSite(rootDir string) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		abs = rootDir
	}
	n.siteRoot = abs
	n.Host.SetStreamHandler(protocol.ID(proto.SiteProtoID), n.handleSiteStream)
}

func (n *Node) handleSiteStream(s network.Stream) {
	defer s.Close()

	in := bufio.NewReader(s)
	line, err := in.ReadString('\n')
	if err != nil {
		return
	}
	line = strings.TrimSpace(line)

	if !strings.HasPrefix(line, "GET ") {
		_, _ = io.WriteString(s, "ERR bad request\n")
		return
	}

	if n.siteRoot == "" {
		_, _ = io.WriteString(s, "ERR site disabled\n")
		return
	}

	reqPath := strings.TrimSpace(strings.TrimPrefix(line, "GET "))
	if reqPath == "" || reqPath == "/" {
		reqPath = "/index.html"
	}

	clean := filepath.Clean(reqPath)
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.TrimPrefix(clean, `\`)

	full := filepath.Join(n.siteRoot, clean)

	rootWithSep := n.siteRoot + string(filepath.Separator)
	if full != n.siteRoot && !strings.HasPrefix(full, rootWithSep) {
		_, _ = io.WriteString(s, "ERR forbidden\n")
		return
	}

	// Block access to lua/ directory (scripts and state)
	if strings.HasPrefix(clean, "lua/") || clean == "lua" {
		_, _ = io.WriteString(s, "ERR forbidden\n")
		return
	}

	b, err := os.ReadFile(full)
	if err != nil {
		_, _ = io.WriteString(s, "ERR not found\n")
		return
	}

	mt := mime.TypeByExtension(filepath.Ext(full))
	if mt == "" {
		mt = http.DetectContentType(b)
	}

	_, _ = fmt.Fprintf(s, "OK %s %d\n", mt, len(b))
	_, _ = s.Write(b)
}

// dialAndOpenStream connects to a peer and opens a SITE protocol stream.
// Returns the addresses that were tried, the open stream (on success), or an error.
func (n *Node) dialAndOpenStream(ctx context.Context, pid peer.ID) (addrStrs []string, st network.Stream, err error) {
	knownAddrs := n.Host.Peerstore().Addrs(pid)
	n.diag("SITE: dialing %s (%d known addrs)", pid, len(knownAddrs))
	for _, a := range knownAddrs {
		s := a.String()
		addrStrs = append(addrStrs, s)
		n.diag("SITE:   addr: %s", s)
	}

	// Clear any dial backoff so we get a fresh connection attempt.
	if sw, ok := n.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(pid)
	}
	if err := n.Host.Connect(ctx, peer.AddrInfo{ID: pid}); err != nil {
		n.diag("SITE: connect failed: %v", err)
		return addrStrs, nil, fmt.Errorf("connect: %w", err)
	}

	st, err = n.Host.NewStream(ctx, pid, protocol.ID(proto.SiteProtoID))
	if err != nil {
		n.diag("SITE: stream open failed: %v", err)
		return addrStrs, nil, fmt.Errorf("stream: %w", err)
	}
	return addrStrs, st, nil
}

// forceRelayRecovery closes all connections to the relay peer and reconnects,
// forcing AutoRelay to obtain a fresh reservation.
func (n *Node) forceRelayRecovery(ctx context.Context) {
	if n.relayPeer == nil {
		return
	}

	// Close existing connections to the relay — this forces a fresh start.
	conns := n.Host.Network().ConnsToPeer(n.relayPeer.ID)
	for _, c := range conns {
		n.diag("SITE: closing stale relay connection: %s", c.RemoteMultiaddr())
		_ = c.Close()
	}

	// Also close connections to the TARGET peer (they may be stale relay streams).
	// The caller will re-dial after recovery.

	// Clear backoff and re-add relay addresses.
	if sw, ok := n.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(n.relayPeer.ID)
	}
	n.Host.Peerstore().AddAddrs(n.relayPeer.ID, n.relayPeer.Addrs, 10*time.Minute)

	// Reconnect to relay.
	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := n.Host.Connect(connCtx, *n.relayPeer); err != nil {
		n.diag("SITE: relay recovery connect failed: %v", err)
		return
	}
	n.diag("SITE: relay recovered, waiting for reservation...")

	// Wait briefly for AutoRelay to re-establish reservation.
	deadline := time.After(8 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			n.diag("SITE: relay reservation timeout")
			return
		case <-tick.C:
			if n.hasCircuitAddr() {
				n.diag("SITE: relay reservation restored")
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) FetchSiteFile(ctx context.Context, peerID string, path string) (mimeType string, data []byte, err error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return "", nil, err
	}

	addrStrs, st, dialErr := n.dialAndOpenStream(ctx, pid)
	if dialErr != nil && n.relayPeer != nil {
		n.diag("SITE: direct dial failed, attempting relay recovery...")

		// Close stale connections to both relay AND target peer.
		for _, c := range n.Host.Network().ConnsToPeer(pid) {
			_ = c.Close()
		}

		// Use a FRESH context — the original is likely expired after
		// the first dial hung for its full timeout.
		retryCtx, retryCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer retryCancel()

		n.forceRelayRecovery(retryCtx)

		// Inject a constructed circuit relay address for the target peer
		// so the retry can route through the relay even if the peer never
		// published a circuit address in its presence messages.
		n.addRelayAddrForPeer(pid)

		addrStrs, st, dialErr = n.dialAndOpenStream(retryCtx, pid)
	}
	if dialErr != nil {
		detail := fmt.Sprintf("peer unreachable\naddrs: %s\nerror: %v", strings.Join(addrStrs, ", "), dialErr)
		return "", nil, fmt.Errorf("%s", detail)
	}
	defer st.Close()

	if path == "" || path == "/" {
		path = "/index.html"
	}
	_, _ = io.WriteString(st, "GET "+path+"\n")

	r := bufio.NewReader(st)
	h, err := r.ReadString('\n')
	if err != nil {
		return "", nil, err
	}
	h = strings.TrimSpace(h)

	if strings.HasPrefix(h, "ERR ") {
		return "", nil, fmt.Errorf("%s", strings.TrimPrefix(h, "ERR "))
	}

	// ---- FIX: parse size from the END ----
	lastSpace := strings.LastIndexByte(h, ' ')
	if lastSpace == -1 || !strings.HasPrefix(h, "OK ") {
		return "", nil, fmt.Errorf("bad response: %q", h)
	}

	mimeType = strings.TrimSpace(h[3:lastSpace])
	sizeStr := strings.TrimSpace(h[lastSpace+1:])

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return "", nil, err
	}
	if size < 0 || size > 50*1024*1024 {
		return "", nil, fmt.Errorf("refusing size %d", size)
	}

	data = make([]byte, size)
	_, err = io.ReadFull(r, data)
	return mimeType, data, err
}
