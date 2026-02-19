// HTTP API endpoints for group document sharing.

package routes

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/docs"
)

func registerDocsRoutes(mux *http.ServeMux, d Deps) {
	if d.DocsStore == nil {
		return
	}

	// List my shared files for a group
	handleGet(mux, "/api/docs/my", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("group_id")
		if groupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}

		files, err := d.DocsStore.List(groupID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list files: %v", err), http.StatusInternalServerError)
			return
		}

		selfID := ""
		if d.Node != nil {
			selfID = d.Node.ID()
		}

		writeJSON(w, map[string]any{
			"files":   files,
			"peer_id": selfID,
		})
	})

	// Upload a file to share with the group
	mux.HandleFunc("/api/docs/upload", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireLocal(w, r) {
			return
		}

		// Parse multipart: max 50 MB + overhead
		if err := r.ParseMultipartForm(docs.MaxFileSize + 1024); err != nil {
			http.Error(w, "File too large or bad form", http.StatusBadRequest)
			return
		}

		groupID := r.FormValue("group_id")
		if groupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		if header.Size > docs.MaxFileSize {
			http.Error(w, "File exceeds 50 MB limit", http.StatusRequestEntityTooLarge)
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, docs.MaxFileSize+1))
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}
		if len(data) > docs.MaxFileSize {
			http.Error(w, "File exceeds 50 MB limit", http.StatusRequestEntityTooLarge)
			return
		}

		hash, err := d.DocsStore.Save(groupID, header.Filename, data)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
			return
		}

		// Broadcast doc-added event to the group via the group relay
		if d.GroupManager != nil {
			payload := map[string]any{
				"action": "doc-added",
				"file": map[string]any{
					"name": header.Filename,
					"size": len(data),
					"hash": hash,
				},
			}
			// Try sending as host first, then as client
			if err := d.GroupManager.SendToGroupAsHost(groupID, payload); err != nil {
				_ = d.GroupManager.SendToGroup(payload)
			}
		}

		writeJSON(w, map[string]any{
			"status":   "uploaded",
			"filename": header.Filename,
			"size":     len(data),
			"hash":     hash,
		})
	})

	// Delete a shared file
	handlePost(mux, "/api/docs/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID  string `json:"group_id"`
		Filename string `json:"filename"`
	}) {
		if !requireLocal(w, r) {
			return
		}
		if req.GroupID == "" || req.Filename == "" {
			http.Error(w, "Missing group_id or filename", http.StatusBadRequest)
			return
		}

		if err := d.DocsStore.Delete(req.GroupID, req.Filename); err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete: %v", err), http.StatusInternalServerError)
			return
		}

		// Broadcast doc-removed event
		if d.GroupManager != nil {
			payload := map[string]any{
				"action": "doc-removed",
				"file":   req.Filename,
			}
			if err := d.GroupManager.SendToGroupAsHost(req.GroupID, payload); err != nil {
				_ = d.GroupManager.SendToGroup(payload)
			}
		}

		writeJSON(w, map[string]string{"status": "deleted"})
	})

	// Browse: aggregate file lists from all group members
	handleGet(mux, "/api/docs/browse", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("group_id")
		if groupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}

		selfID := ""
		if d.Node != nil {
			selfID = d.Node.ID()
		}

		type peerFiles struct {
			PeerID string         `json:"peer_id"`
			Label  string         `json:"label"`
			Files  []docs.DocInfo `json:"files"`
			Self   bool           `json:"self"`
			Error  string         `json:"error,omitempty"`
		}

		var results []peerFiles

		// My own files
		myFiles, err := d.DocsStore.List(groupID)
		if err != nil {
			myFiles = []docs.DocInfo{}
		}
		myLabel := ""
		if d.SelfLabel != nil {
			myLabel = d.SelfLabel()
		}
		if myLabel == "" {
			myLabel = "Me"
		}
		results = append(results, peerFiles{
			PeerID: selfID,
			Label:  myLabel,
			Files:  myFiles,
			Self:   true,
		})

		// Query group members for their files
		if d.Node != nil && d.GroupManager != nil {
			// Collect all known member peer IDs from every source, deduplicating.
			// StoredGroupMembers is the key fallback: it works even when the host is offline.
			seen := map[string]bool{selfID: true}
			var peerIDs []string
			addPeer := func(pid string) {
				if !seen[pid] {
					seen[pid] = true
					peerIDs = append(peerIDs, pid)
				}
			}
			for _, m := range d.GroupManager.HostedGroupMembers(groupID) {
				addPeer(m.PeerID)
			}
			for _, m := range d.GroupManager.ClientGroupMembers(groupID) {
				addPeer(m.PeerID)
			}
			for _, pid := range d.GroupManager.StoredGroupMembers(groupID) {
				addPeer(pid)
			}

			// Query each peer in parallel
			if len(peerIDs) > 0 {
				var mu sync.Mutex
				var wg sync.WaitGroup
				for _, pid := range peerIDs {
					wg.Add(1)
					go func(peerID string) {
						defer wg.Done()
						ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
						defer cancel()

						files, err := d.Node.FetchDocList(ctx, peerID, groupID)

						// Resolve peer label from peer table
						label := peerID
						if d.Peers != nil {
							snap := d.Peers.Snapshot()
							if sp, ok := snap[peerID]; ok && sp.Content != "" {
								label = sp.Content
							}
						}

						pf := peerFiles{
							PeerID: peerID,
							Label:  label,
							Self:   false,
						}
						if err != nil {
							log.Printf("DOCS: Failed to fetch list from %s: %v", peerID, err)
							pf.Error = err.Error()
							pf.Files = []docs.DocInfo{}
						} else {
							if files == nil {
								files = []docs.DocInfo{}
							}
							pf.Files = files
						}

						mu.Lock()
						results = append(results, pf)
						mu.Unlock()
					}(pid)
				}
				wg.Wait()
			}
		}

		writeJSON(w, map[string]any{
			"group_id": groupID,
			"peers":    results,
		})
	})

	// Download a file (from own store or proxy from remote peer)
	handleGet(mux, "/api/docs/download", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer_id")
		groupID := r.URL.Query().Get("group_id")
		filename := r.URL.Query().Get("file")

		if groupID == "" || filename == "" {
			http.Error(w, "Missing group_id or file", http.StatusBadRequest)
			return
		}

		selfID := ""
		if d.Node != nil {
			selfID = d.Node.ID()
		}

		// If peer_id is empty or is self, serve from local store
		if peerID == "" || peerID == selfID {
			data, _, err := d.DocsStore.Read(groupID, filename)
			if err != nil {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}
			ct := http.DetectContentType(data)
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.Write(data)
			return
		}

		// Fetch from remote peer
		if d.Node == nil {
			http.Error(w, "P2P not available", http.StatusServiceUnavailable)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		mimeType, data, err := d.Node.FetchDocFile(ctx, peerID, groupID, filename)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to fetch: %v", err), http.StatusBadGateway)
			return
		}

		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
	})
}
