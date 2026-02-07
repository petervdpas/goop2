// internal/p2p/docs.go
// libp2p stream protocol for group document sharing.
// Peers can list and fetch documents from other group members.

package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/petervdpas/goop2/internal/docs"
	"github.com/petervdpas/goop2/internal/proto"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// GroupChecker is the interface used to verify group membership.
type GroupChecker interface {
	IsPeerInGroup(peerID, groupID string) bool
	IsGroupHost(groupID string) bool
}

// EnableDocs registers the docs stream handler.
func (n *Node) EnableDocs(store *docs.Store, gc GroupChecker) {
	n.docsStore = store
	n.groupChecker = gc
	n.Host.SetStreamHandler(protocol.ID(proto.DocsProtoID), n.handleDocsStream)
}

// docsRequest is the wire format for incoming doc requests.
type docsRequest struct {
	Op      string `json:"op"`       // "list" or "get"
	GroupID string `json:"group_id"`
	File    string `json:"file,omitempty"` // for "get"
}

// docsListResponse is the response for a "list" operation.
type docsListResponse struct {
	OK    bool        `json:"ok"`
	Files []docs.DocInfo `json:"files,omitempty"`
	Error string      `json:"error,omitempty"`
}

func (n *Node) handleDocsStream(s network.Stream) {
	defer s.Close()

	if n.docsStore == nil {
		writeDocsError(s, "docs not enabled")
		return
	}

	remotePeer := s.Conn().RemotePeer().String()

	dec := json.NewDecoder(s)
	var req docsRequest
	if err := dec.Decode(&req); err != nil {
		writeDocsError(s, "bad request")
		return
	}

	if req.GroupID == "" {
		writeDocsError(s, "missing group_id")
		return
	}

	// Access control: verify the requesting peer is in this group
	if n.groupChecker != nil && n.groupChecker.IsGroupHost(req.GroupID) {
		if !n.groupChecker.IsPeerInGroup(remotePeer, req.GroupID) {
			log.Printf("DOCS: Access denied for %s on group %s", remotePeer, req.GroupID)
			writeDocsError(s, "access denied: not a group member")
			return
		}
	}

	switch req.Op {
	case "list":
		n.handleDocsList(s, req)
	case "get":
		n.handleDocsGet(s, req)
	default:
		writeDocsError(s, "unknown op: "+req.Op)
	}
}

func (n *Node) handleDocsList(s network.Stream, req docsRequest) {
	files, err := n.docsStore.List(req.GroupID)
	if err != nil {
		writeDocsError(s, "list failed: "+err.Error())
		return
	}

	resp := docsListResponse{OK: true, Files: files}
	json.NewEncoder(s).Encode(resp)
}

func (n *Node) handleDocsGet(s network.Stream, req docsRequest) {
	if req.File == "" {
		writeDocsError(s, "missing file")
		return
	}

	data, _, err := n.docsStore.Read(req.GroupID, req.File)
	if err != nil {
		writeDocsError(s, "not found")
		return
	}

	if len(data) > docs.MaxFileSize {
		writeDocsError(s, "file too large")
		return
	}

	mt := mime.TypeByExtension(filepath.Ext(req.File))
	if mt == "" {
		mt = http.DetectContentType(data)
	}

	// Write response header (same format as site.go)
	fmt.Fprintf(s, "OK %s %d\n", mt, len(data))
	s.Write(data)
}

func writeDocsError(s network.Stream, msg string) {
	resp := docsListResponse{OK: false, Error: msg}
	json.NewEncoder(s).Encode(resp)
}

// FetchDocList retrieves the file list from a remote peer for a group.
func (n *Node) FetchDocList(ctx context.Context, peerID, groupID string) ([]docs.DocInfo, error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID: %w", err)
	}

	_ = n.Host.Connect(ctx, peer.AddrInfo{ID: pid})

	st, err := n.Host.NewStream(ctx, pid, protocol.ID(proto.DocsProtoID))
	if err != nil {
		return nil, fmt.Errorf("failed to open docs stream: %w", err)
	}
	defer st.Close()

	req := docsRequest{Op: "list", GroupID: groupID}
	if err := json.NewEncoder(st).Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Signal we're done writing
	if closer, ok := st.(interface{ CloseWrite() error }); ok {
		closer.CloseWrite()
	}

	var resp docsListResponse
	if err := json.NewDecoder(st).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if !resp.OK {
		return nil, fmt.Errorf("remote error: %s", resp.Error)
	}

	return resp.Files, nil
}

// FetchDocFile downloads a file from a remote peer.
func (n *Node) FetchDocFile(ctx context.Context, peerID, groupID, filename string) (string, []byte, error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid peer ID: %w", err)
	}

	_ = n.Host.Connect(ctx, peer.AddrInfo{ID: pid})

	st, err := n.Host.NewStream(ctx, pid, protocol.ID(proto.DocsProtoID))
	if err != nil {
		return "", nil, fmt.Errorf("failed to open docs stream: %w", err)
	}
	defer st.Close()

	req := docsRequest{Op: "get", GroupID: groupID, File: filename}
	if err := json.NewEncoder(st).Encode(req); err != nil {
		return "", nil, fmt.Errorf("failed to send request: %w", err)
	}

	if closer, ok := st.(interface{ CloseWrite() error }); ok {
		closer.CloseWrite()
	}

	r := bufio.NewReader(st)

	// Try to read first byte to detect JSON error vs OK response
	firstByte, err := r.ReadByte()
	if err != nil {
		return "", nil, fmt.Errorf("failed to read response: %w", err)
	}

	if firstByte == '{' {
		// JSON error response
		rest, _ := io.ReadAll(r)
		full := append([]byte{firstByte}, rest...)
		var resp docsListResponse
		if err := json.Unmarshal(full, &resp); err != nil {
			return "", nil, fmt.Errorf("bad response: %s", string(full))
		}
		return "", nil, fmt.Errorf("remote error: %s", resp.Error)
	}

	// Put byte back and read the OK header line
	r.UnreadByte()
	h, err := r.ReadString('\n')
	if err != nil {
		return "", nil, fmt.Errorf("failed to read header: %w", err)
	}
	h = strings.TrimSpace(h)

	if strings.HasPrefix(h, "ERR ") {
		return "", nil, fmt.Errorf("%s", strings.TrimPrefix(h, "ERR "))
	}

	lastSpace := strings.LastIndexByte(h, ' ')
	if lastSpace == -1 || !strings.HasPrefix(h, "OK ") {
		return "", nil, fmt.Errorf("bad response: %q", h)
	}

	mimeType := strings.TrimSpace(h[3:lastSpace])
	sizeStr := strings.TrimSpace(h[lastSpace+1:])

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return "", nil, fmt.Errorf("bad size: %w", err)
	}
	if size < 0 || size > docs.MaxFileSize {
		return "", nil, fmt.Errorf("refusing size %d", size)
	}

	data := make([]byte, size)
	_, err = io.ReadFull(r, data)
	return mimeType, data, err
}
