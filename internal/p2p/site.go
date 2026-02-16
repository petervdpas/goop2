
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
		// Log which connections exist — helps diagnose when NewStream
		// picks a broken connection over a working one.
		conns := n.Host.Network().ConnsToPeer(pid)
		for _, c := range conns {
			n.diag("SITE:   conn: %s (%s, %d streams)",
				c.RemoteMultiaddr(), dirString(c.Stat().Direction), len(c.GetStreams()))
		}
		n.diag("SITE: stream open failed: %v", err)
		return addrStrs, nil, fmt.Errorf("stream: %w", err)
	}
	n.diag("SITE: stream opened via %s", st.Conn().RemoteMultiaddr())
	return addrStrs, st, nil
}

func (n *Node) FetchSiteFile(ctx context.Context, peerID string, path string) (mimeType string, data []byte, err error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return "", nil, err
	}

	addrStrs, st, dialErr := n.dialAndOpenStream(ctx, pid)
	if dialErr != nil && n.relayPeer != nil {
		n.diag("SITE: dial failed for %s, falling back to relay circuit", pid.ShortString())

		// Fresh context — the original likely expired during the dial.
		retryCtx, retryCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer retryCancel()

		// Pulse the target peer via rendezvous — tells it to refresh
		// its relay reservation. This is the primary recovery mechanism.
		if n.pulseFn != nil {
			if err := n.pulseFn(retryCtx, peerID); err != nil {
				n.diag("SITE: pulse failed: %v", err)
			} else {
				n.diag("SITE: pulse succeeded for %s", pid.ShortString())
			}
		}

		// Retry with per-attempt timeout — prevents one hanging
		// NewStream() from consuming the entire retry budget.
		for attempt := 1; attempt <= 3; attempt++ {
			// Before each attempt: close connections that may have been
			// (re-)established by background activity (presence heartbeats,
			// identify rounds) and purge ALL peerstore addresses. Then
			// inject ONLY the circuit relay address — forcing the dial
			// through the relay. Without this, Host.Connect() may
			// re-establish a broken direct TCP connection that NewStream()
			// then picks over the working circuit.
			for _, c := range n.Host.Network().ConnsToPeer(pid) {
				_ = c.Close()
			}
			n.Host.Peerstore().ClearAddrs(pid)
			n.addRelayAddrForPeer(pid)

			attemptCtx, attemptCancel := context.WithTimeout(retryCtx, 15*time.Second)
			addrStrs, st, dialErr = n.dialAndOpenStream(attemptCtx, pid)
			attemptCancel()
			if dialErr == nil {
				break
			}
			n.diag("SITE: relay retry %d/3 failed for %s: %v", attempt, pid.ShortString(), dialErr)
			if attempt < 3 {
				select {
				case <-retryCtx.Done():
					break
				case <-time.After(time.Duration(attempt*3) * time.Second):
				}
			}
		}
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
