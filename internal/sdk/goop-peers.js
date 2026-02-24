//
// Peer discovery client for site pages.
// Usage:
//
//   <script src="/sdk/goop-peers.js"></script>
//
//   // list online peers
//   const peers = await Goop.peers.list();
//   // Each peer object:
//   // {
//   //   ID:             string   — peer ID
//   //   Content:        string   — display name / profile text
//   //   Email:          string   — contact email (may be empty)
//   //   AvatarHash:     string   — use /api/avatar/peer/<ID> to fetch the image
//   //   VideoDisabled:  bool     — peer has disabled incoming video calls
//   //   ActiveTemplate: string   — name of the peer's active site template
//   //   Verified:       bool     — peer identity is verified
//   //   Reachable:      bool     — peer is currently reachable over the network
//   //   Offline:        bool     — peer has gone offline
//   //   LastSeen:       string   — RFC3339 timestamp of last activity
//   // }
//
//   // subscribe to live updates (polls /api/peers every 5 s by default)
//   Goop.peers.subscribe({
//     onSnapshot(peers)         { /* full list on first load, same shape as list() */ },
//     onUpdate(peer_id, peer)   { /* peer came online, updated, or reachability changed */ },
//     onRemove(peer_id)         { /* peer was pruned from the list */ },
//   }, 5000 /* optional poll interval ms */);
//
//   // unsubscribe
//   Goop.peers.unsubscribe();
//
(() => {
  window.Goop = window.Goop || {};

  let _timer = null;

  window.Goop.peers = {
    /** Fetch current peer list. Returns array of peer objects (see file header for fields). */
    list() {
      return fetch("/api/peers").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /**
     * Subscribe to live peer updates via REST polling.
     * callbacks: { onSnapshot(peers), onUpdate(peer_id, peer), onRemove(peer_id) }
     * pollIntervalMs defaults to 5000.
     */
    subscribe(callbacks, pollIntervalMs) {
      if (_timer) { clearInterval(_timer); _timer = null; }

      const cb = callbacks || {};
      const interval = pollIntervalMs || 5000;
      let known = null; // null until first successful fetch

      function poll() {
        fetch("/api/peers")
          .then((r) => { if (!r.ok) throw new Error(r.statusText); return r.json(); })
          .then((peers) => {
            if (!Array.isArray(peers)) return;

            if (known === null) {
              // First load: fire onSnapshot with the full list.
              known = {};
              peers.forEach((p) => { known[p.ID] = p; });
              if (cb.onSnapshot) cb.onSnapshot(peers);
              return;
            }

            // Subsequent polls: diff against known set.
            const next = {};
            peers.forEach((p) => {
              next[p.ID] = p;
              const prev = known[p.ID];
              if (!prev || JSON.stringify(prev) !== JSON.stringify(p)) {
                if (cb.onUpdate) cb.onUpdate(p.ID, p);
              }
            });
            Object.keys(known).forEach((id) => {
              if (!next[id] && cb.onRemove) cb.onRemove(id);
            });
            known = next;
          })
          .catch(() => { if (cb.onError) cb.onError(); });
      }

      poll(); // immediate first fetch
      _timer = setInterval(poll, interval);
    },

    /** Stop polling */
    unsubscribe() {
      if (_timer) { clearInterval(_timer); _timer = null; }
    },
  };
})();
