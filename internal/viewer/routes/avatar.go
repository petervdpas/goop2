package routes

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/avatar"
)

func registerAvatarRoutes(mux *http.ServeMux, d Deps) {
	// POST /api/avatar/upload — upload own avatar (multipart, max 256KB)
	mux.HandleFunc("/api/avatar/upload", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireLocal(w, r) {
			return
		}
		if d.AvatarStore == nil {
			http.Error(w, "avatar store not configured", http.StatusInternalServerError)
			return
		}

		// Limit body to 256KB + overhead
		r.Body = http.MaxBytesReader(w, r.Body, 300*1024)

		if err := r.ParseMultipartForm(300 * 1024); err != nil {
			http.Error(w, "file too large (max 256KB)", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("avatar")
		if err != nil {
			http.Error(w, "missing avatar field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(io.LimitReader(file, 256*1024+1))
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		if len(data) > 256*1024 {
			http.Error(w, "file too large (max 256KB)", http.StatusBadRequest)
			return
		}

		if err := d.AvatarStore.Write(data); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"hash":"` + d.AvatarStore.Hash() + `"}`))
	})

	// DELETE /api/avatar — delete own avatar
	mux.HandleFunc("/api/avatar/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !requireLocal(w, r) {
			return
		}
		if d.AvatarStore == nil {
			http.Error(w, "avatar store not configured", http.StatusInternalServerError)
			return
		}

		if err := d.AvatarStore.Delete(); err != nil {
			http.Error(w, "delete error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	// GET /api/avatar — serve own avatar (or initials SVG fallback)
	mux.HandleFunc("/api/avatar", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		// If path has an extra segment, it's a peer avatar request
		rest := strings.TrimPrefix(r.URL.Path, "/api/avatar")
		rest = strings.TrimPrefix(rest, "/")
		if rest != "" && rest != "upload" && rest != "delete" {
			serveRemoteAvatar(w, r, d, rest)
			return
		}

		if d.AvatarStore == nil {
			serveFallbackSVG(w, safeCall(d.SelfLabel), safeCall(d.SelfEmail))
			return
		}

		data, err := d.AvatarStore.Read()
		if err != nil || data == nil {
			serveFallbackSVG(w, safeCall(d.SelfLabel), safeCall(d.SelfEmail))
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	})

	// GET /api/avatar/{peerID} — serve a remote peer's avatar
	mux.HandleFunc("/api/avatar/peer/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		peerID := strings.TrimPrefix(r.URL.Path, "/api/avatar/peer/")
		if peerID == "" {
			http.Error(w, "missing peer id", http.StatusBadRequest)
			return
		}
		serveRemoteAvatar(w, r, d, peerID)
	})
}

func serveRemoteAvatar(w http.ResponseWriter, r *http.Request, d Deps, peerID string) {
	if d.AvatarCache == nil || d.Peers == nil {
		servePeerFallback(w, d, peerID)
		return
	}

	sp, ok := d.Peers.Get(peerID)
	if !ok || sp.AvatarHash == "" {
		servePeerFallback(w, d, peerID)
		return
	}

	// Check disk cache
	cached, err := d.AvatarCache.Get(peerID, sp.AvatarHash)
	if err == nil && cached != nil {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(cached)
		return
	}

	// Fetch via P2P
	if d.Node == nil {
		servePeerFallback(w, d, peerID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	data, err := d.Node.FetchAvatar(ctx, peerID)
	if err != nil || data == nil {
		servePeerFallback(w, d, peerID)
		return
	}

	// Cache to disk
	_ = d.AvatarCache.Put(peerID, sp.AvatarHash, data)

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(data)
}

func servePeerFallback(w http.ResponseWriter, d Deps, peerID string) {
	label := ""
	email := ""
	if d.Peers != nil {
		if sp, ok := d.Peers.Get(peerID); ok {
			label = sp.Content
			email = sp.Email
		}
	}
	if label == "" {
		label = peerID
	}
	serveFallbackSVG(w, label, email)
}

func serveFallbackSVG(w http.ResponseWriter, label, email string) {
	svg := avatar.InitialsSVG(label, email)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(svg)
}
