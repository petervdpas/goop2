// HTTP API endpoints for group document sharing.

package routes

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"encoding/json"

	files "github.com/petervdpas/goop2/internal/group_types/files"
)

func registerDocsRoutes(mux *http.ServeMux, d Deps) {
	if d.DocsStore == nil {
		return
	}

	// List all file groups with their local files included.
	// Sources: hosted groups (group_type "files"), subscribed groups, and
	// groups that still have files on disk after leaving.
	handleGet(mux, "/api/docs/groups", func(w http.ResponseWriter, r *http.Request) {
		type docGroup struct {
			GroupID   string         `json:"group_id"`
			GroupName string         `json:"group_name"`
			Source    string         `json:"source"` // "hosted", "subscribed", "local"
			Files     []files.DocInfo `json:"files"`
		}

		seen := map[string]*docGroup{}

		if d.GroupManager != nil {
			if hosted, err := d.GroupManager.ListHostedGroups(); err == nil {
				for _, g := range hosted {
					if g.GroupType == "files" {
						seen[g.ID] = &docGroup{GroupID: g.ID, GroupName: g.Name, Source: "hosted"}
					}
				}
			}
			if subs, err := d.GroupManager.ListSubscriptions(); err == nil {
				for _, s := range subs {
					if s.GroupType == "files" {
						if _, ok := seen[s.GroupID]; !ok {
							seen[s.GroupID] = &docGroup{GroupID: s.GroupID, GroupName: s.GroupName, Source: "subscribed"}
						}
					}
				}
			}
		}

		if diskGroups, err := d.DocsStore.ListGroups(); err == nil {
			for _, gid := range diskGroups {
				if _, ok := seen[gid]; !ok {
					seen[gid] = &docGroup{GroupID: gid, Source: "local"}
				}
			}
		}

		// Populate each group with its local files
		for _, g := range seen {
			docFiles, err := d.DocsStore.List(g.GroupID)
			if err != nil || docFiles == nil {
				g.Files = []files.DocInfo{}
			} else {
				g.Files = docFiles
			}
		}

		groups := make([]docGroup, 0, len(seen))
		for _, g := range seen {
			groups = append(groups, *g)
		}

		writeJSON(w, map[string]any{"groups": groups})
	})

	// List my shared files for a group
	handleGet(mux, "/api/docs/my", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("group_id")
		if groupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}

		docFiles, err := d.DocsStore.List(groupID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list files: %v", err), http.StatusInternalServerError)
			return
		}

		selfID := ""
		if d.Node != nil {
			selfID = d.Node.ID()
		}

		writeJSON(w, map[string]any{
			"files":   docFiles,
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
		if err := r.ParseMultipartForm(files.MaxFileSize + 1024); err != nil {
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

		if header.Size > files.MaxFileSize {
			http.Error(w, "File exceeds 50 MB limit", http.StatusRequestEntityTooLarge)
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, files.MaxFileSize+1))
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}
		if len(data) > files.MaxFileSize {
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
				_ = d.GroupManager.SendToGroup(groupID, payload)
			}
		}

		writeJSON(w, map[string]any{
			"status":   "uploaded",
			"filename": header.Filename,
			"size":     len(data),
			"hash":     hash,
		})
	})

	// Upload a file from a local filesystem path
	handlePost(mux, "/api/docs/upload-local", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
		Path    string `json:"path"`
	}) {
		if !requireLocal(w, r) {
			return
		}
		if req.GroupID == "" || req.Path == "" {
			http.Error(w, "Missing group_id or path", http.StatusBadRequest)
			return
		}
		data, err := os.ReadFile(req.Path)
		if err != nil {
			http.Error(w, fmt.Sprintf("Cannot read file: %v", err), http.StatusBadRequest)
			return
		}
		if len(data) > files.MaxFileSize {
			http.Error(w, "File exceeds 50 MB limit", http.StatusRequestEntityTooLarge)
			return
		}
		filename := filepath.Base(req.Path)
		hash, err := d.DocsStore.Save(req.GroupID, filename, data)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
			return
		}
		if d.GroupManager != nil {
			payload := map[string]any{
				"action": "doc-added",
				"file": map[string]any{
					"name": filename,
					"size": len(data),
					"hash": hash,
				},
			}
			if err := d.GroupManager.SendToGroupAsHost(req.GroupID, payload); err != nil {
				_ = d.GroupManager.SendToGroup(req.GroupID, payload)
			}
		}
		writeJSON(w, map[string]any{
			"status":   "uploaded",
			"filename": filename,
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
				_ = d.GroupManager.SendToGroup(req.GroupID, payload)
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
			Files  []files.DocInfo `json:"files"`
			Self   bool           `json:"self"`
			Error  string         `json:"error,omitempty"`
		}

		var results []peerFiles

		// My own files
		myFiles, err := d.DocsStore.List(groupID)
		if err != nil {
			myFiles = []files.DocInfo{}
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
			for _, gm := range d.GroupManager.StoredGroupMembers(groupID) {
				addPeer(gm.PeerID)
			}

			// Query each peer in parallel
			if len(peerIDs) > 0 {
				var mu sync.Mutex
				var wg sync.WaitGroup
				for _, pid := range peerIDs {
					wg.Add(1)
					go func(peerID string) {
						defer wg.Done()
						ctx, cancel := context.WithTimeout(context.Background(), DocListFetchTimeout)
						defer cancel()

						rawFiles, err := d.Node.FetchDocList(ctx, peerID, groupID)

						// Resolve peer label: live table first, persistent cache as fallback
						label := peerID
						if d.ResolvePeer != nil {
							if n := d.ResolvePeer(peerID).Name; n != "" {
								label = n
							}
						}

						pf := peerFiles{
							PeerID: peerID,
							Label:  label,
							Self:   false,
						}
						if err != nil {
							log.Printf("DOCS: Failed to fetch list from %s: %v", peerID, err)
							pf.Error = "unreachable"
							pf.Files = []files.DocInfo{}
						} else {
							var parsed []files.DocInfo
							if rawFiles != nil {
								json.Unmarshal(rawFiles, &parsed) //nolint:errcheck
							}
							if parsed == nil {
								parsed = []files.DocInfo{}
							}
							pf.Files = parsed
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

	// Download a file (from own store or proxy from remote peer).
	// Pass ?inline=1 to serve with Content-Disposition: inline (for browser preview).
	handleGet(mux, "/api/docs/download", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer_id")
		groupID := r.URL.Query().Get("group_id")
		filename := r.URL.Query().Get("file")

		if groupID == "" || filename == "" {
			http.Error(w, "Missing group_id or file", http.StatusBadRequest)
			return
		}

		disposition := "attachment"
		if r.URL.Query().Get("inline") == "1" {
			disposition = "inline"
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
			w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.Write(data)
			return
		}

		// Fetch from remote peer
		if d.Node == nil {
			http.Error(w, "P2P not available", http.StatusServiceUnavailable)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), DocFileFetchTimeout)
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
		w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
	})
}
