// internal/p2p/site.go

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

	"goop/internal/proto"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
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

func (n *Node) FetchSiteFile(ctx context.Context, peerID string, path string) (mimeType string, data []byte, err error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return "", nil, err
	}

	_ = n.Host.Connect(ctx, peer.AddrInfo{ID: pid})

	st, err := n.Host.NewStream(ctx, pid, protocol.ID(proto.SiteProtoID))
	if err != nil {
		return "", nil, err
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
