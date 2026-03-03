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
	IsGroupHost(groupID string) bool
	IsPeerInGroup(peerID, groupID string) bool
	IsTemplateMember(peerID string) bool
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

	// Read request — may be plaintext JSON or encrypted ENC: line
	rd := bufio.NewReader(s)
	line, err := rd.ReadBytes('\n')
	if err != nil && err != io.EOF {
		writeDocsError(s, "bad request")
		return
	}

	jsonLine := line
	if n.enc != nil && len(line) > 4 && string(line[:4]) == "ENC:" {
		trimmed := strings.TrimSpace(string(line[4:]))
		plaintext, err := n.enc.Open(remotePeer, trimmed)
		if err != nil {
			writeDocsError(s, "decrypt error")
			return
		}
		jsonLine = plaintext
	}

	var req docsRequest
	if err := json.Unmarshal(jsonLine, &req); err != nil {
		writeDocsError(s, "bad request")
		return
	}

	if req.GroupID == "" {
		writeDocsError(s, "missing group_id")
		return
	}

	// Access control: only the host has an authoritative member list.
	// Non-host peers cannot verify membership so they serve openly.
	if n.groupChecker != nil && n.groupChecker.IsGroupHost(req.GroupID) {
		if !n.groupChecker.IsPeerInGroup(remotePeer, req.GroupID) {
			log.Printf("DOCS: Access denied for %s on group %s", remotePeer, req.GroupID)
			writeDocsError(s, "access denied: not a group member")
			return
		}
	}

	switch req.Op {
	case "list":
		n.handleDocsList(s, remotePeer, req)
	case "get":
		n.handleDocsGet(s, remotePeer, req)
	default:
		writeDocsError(s, "unknown op: "+req.Op)
	}
}

func (n *Node) handleDocsList(s network.Stream, remotePeer string, req docsRequest) {
	files, err := n.docsStore.List(req.GroupID)
	if err != nil {
		writeDocsError(s, "list failed: "+err.Error())
		return
	}

	resp := docsListResponse{OK: true, Files: files}
	b, _ := json.Marshal(resp)
	if n.enc != nil {
		if sealed, err := n.enc.Seal(remotePeer, b); err == nil {
			s.Write([]byte("ENC:" + sealed + "\n"))
			return
		}
	}
	b = append(b, '\n')
	s.Write(b)
}

func (n *Node) handleDocsGet(s network.Stream, remotePeer string, req docsRequest) {
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

	// Encrypt binary response if possible
	if n.enc != nil {
		if sealed, err := n.enc.Seal(remotePeer, data); err == nil {
			sealedBytes := []byte(sealed)
			fmt.Fprintf(s, "EOK %s %d\n", mt, len(sealedBytes))
			s.Write(sealedBytes)
			return
		}
	}

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
	reqJSON, _ := json.Marshal(req)
	if n.enc != nil {
		if sealed, err := n.enc.Seal(peerID, reqJSON); err == nil {
			reqJSON = []byte("ENC:" + sealed)
		}
	}
	reqJSON = append(reqJSON, '\n')
	if _, err := st.Write(reqJSON); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Signal we're done writing
	if closer, ok := st.(interface{ CloseWrite() error }); ok {
		closer.CloseWrite()
	}

	// Read response — may be ENC: or plain JSON
	rd := bufio.NewReader(st)
	respLine, err := rd.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	jsonLine := respLine
	if n.enc != nil && len(respLine) > 4 && string(respLine[:4]) == "ENC:" {
		trimmed := strings.TrimSpace(string(respLine[4:]))
		if plaintext, err := n.enc.Open(peerID, trimmed); err == nil {
			jsonLine = plaintext
		}
	}

	var resp docsListResponse
	if err := json.Unmarshal(jsonLine, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
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
	reqJSON, _ := json.Marshal(req)
	if n.enc != nil {
		if sealed, err := n.enc.Seal(peerID, reqJSON); err == nil {
			reqJSON = []byte("ENC:" + sealed)
		}
	}
	reqJSON = append(reqJSON, '\n')
	if _, err := st.Write(reqJSON); err != nil {
		return "", nil, fmt.Errorf("failed to send request: %w", err)
	}

	if closer, ok := st.(interface{ CloseWrite() error }); ok {
		closer.CloseWrite()
	}

	r := bufio.NewReader(st)

	// Try to read first byte to detect JSON error vs OK/EOK response
	firstByte, err := r.ReadByte()
	if err != nil {
		return "", nil, fmt.Errorf("failed to read response: %w", err)
	}

	// ENC: prefix means encrypted JSON error/list response
	if firstByte == 'E' {
		r.UnreadByte()
		h, err := r.ReadString('\n')
		if err != nil {
			return "", nil, fmt.Errorf("failed to read header: %w", err)
		}
		h = strings.TrimSpace(h)

		// ENC: encrypted JSON response (list or error)
		if strings.HasPrefix(h, "ENC:") {
			if n.enc == nil {
				return "", nil, fmt.Errorf("encrypted response but no decryptor")
			}
			plaintext, err := n.enc.Open(peerID, h[4:])
			if err != nil {
				return "", nil, fmt.Errorf("decrypt response: %w", err)
			}
			var resp docsListResponse
			if err := json.Unmarshal(plaintext, &resp); err != nil {
				return "", nil, fmt.Errorf("decode decrypted response: %w", err)
			}
			if !resp.OK {
				return "", nil, fmt.Errorf("remote error: %s", resp.Error)
			}
			// This path is only hit in FetchDocFile, where we expect binary —
			// an encrypted JSON OK response here means the server sent a list
			// response (wrong op). Return a clear error.
			return "", nil, fmt.Errorf("unexpected list response for get request")
		}

		// EOK header — encrypted binary data
		if strings.HasPrefix(h, "EOK ") {
			lastSpace := strings.LastIndexByte(h, ' ')
			if lastSpace <= 4 {
				return "", nil, fmt.Errorf("bad EOK response: %q", h)
			}
			mimeType := strings.TrimSpace(h[4:lastSpace])
			sizeStr := strings.TrimSpace(h[lastSpace+1:])
			size, err := strconv.Atoi(sizeStr)
			if err != nil {
				return "", nil, fmt.Errorf("bad size: %w", err)
			}
			if size < 0 || size > docs.MaxFileSize*2 {
				return "", nil, fmt.Errorf("refusing size %d", size)
			}
			sealedData := make([]byte, size)
			if _, err := io.ReadFull(r, sealedData); err != nil {
				return "", nil, fmt.Errorf("read encrypted data: %w", err)
			}
			if n.enc != nil {
				if plaintext, err := n.enc.Open(peerID, string(sealedData)); err == nil {
					return mimeType, plaintext, nil
				}
			}
			return "", nil, fmt.Errorf("encrypted data could not be decrypted")
		}

		// ERR response
		if after, ok := strings.CutPrefix(h, "ERR "); ok {
			return "", nil, fmt.Errorf("%s", after)
		}

		return "", nil, fmt.Errorf("bad response: %q", h)
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

	if after, ok := strings.CutPrefix(h, "ERR "); ok {
		return "", nil, fmt.Errorf("%s", after)
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
