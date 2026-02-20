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
//   // subscribe to live updates
//   Goop.peers.subscribe({
//     onSnapshot(peers)   { /* full list, same shape as list() */ },
//     onUpdate(peer)      { /* peer came online or updated */ },
//     onRemove(peer)      { /* peer went offline — only {ID} is present */ },
//   });
//
//   // unsubscribe
//   Goop.peers.unsubscribe();
//
(() => {
  window.Goop = window.Goop || {};

  let sse = null;

  window.Goop.peers = {
    /** Fetch current peer list. Returns array of peer objects (see file header for fields). */
    list() {
      return fetch("/api/peers").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /**
     * Subscribe to live peer updates via SSE.
     * callbacks: { onSnapshot(peers), onUpdate(peer), onRemove(peer) }
     */
    subscribe(callbacks) {
      if (sse) sse.close();

      sse = new EventSource("/api/peers/events");
      const cb = callbacks || {};

      sse.addEventListener("snapshot", (e) => {
        try {
          const data = JSON.parse(e.data);
          if (cb.onSnapshot) cb.onSnapshot(data.peers || []);
        } catch (_) {}
      });

      sse.addEventListener("update", (e) => {
        try {
          if (cb.onUpdate) cb.onUpdate(JSON.parse(e.data));
        } catch (_) {}
      });

      sse.addEventListener("remove", (e) => {
        try {
          if (cb.onRemove) cb.onRemove(JSON.parse(e.data));
        } catch (_) {}
      });

      sse.onerror = () => {
        if (cb.onError) cb.onError();
      };

      return sse;
    },

    /** Close the SSE connection */
    unsubscribe() {
      if (sse) {
        sse.close();
        sse = null;
      }
    },
  };
})();
