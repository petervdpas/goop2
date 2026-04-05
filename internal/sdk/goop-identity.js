//
// Identity client for site pages.
// Usage:
//
//   <script src="/sdk/goop-identity.js"></script>
//
//   // get your peer info
//   const me = await Goop.identity.get();
//   // => {id: "12D3Koo...", label: "peer-C", email: "you@example.com"}
//
//   // shorthand helpers
//   const id    = await Goop.identity.id();
//   const label = await Goop.identity.label();
//   const email = await Goop.identity.email();
//
(() => {
  window.Goop = window.Goop || {};

  let cached = null;

  async function load() {
    if (cached) return cached;
    const res = await fetch("/api/self");
    if (!res.ok) throw new Error(res.statusText);
    cached = await res.json();
    return cached;
  }

  window.Goop.identity = {
    /** Fetch full identity object {id, label, email} */
    get() {
      return load();
    },

    /** Get just the peer ID string */
    async id() {
      const me = await load();
      return me.id;
    },

    /** Get just the peer label string */
    async label() {
      const me = await load();
      return me.label;
    },

    /** Get just the peer email string */
    async email() {
      const me = await load();
      return me.email;
    },

    /** Clear cached identity (forces re-fetch on next call) */
    refresh() {
      cached = null;
    },

    /**
     * Resolve a peer ID to a display name.
     * Checks: self → MQ peer cache → server-provided name → truncated ID.
     * This is THE single name resolver for templates.
     */
    resolveName(peerId, serverName) {
      if (cached && peerId === cached.id) return "You";
      if (serverName) return serverName;
      if (window.Goop.mq && window.Goop.mq.getPeerName) {
        const mqName = window.Goop.mq.getPeerName(peerId);
        if (mqName) return mqName;
      }
      return peerId ? peerId.slice(-6) : "???";
    },
  };
})();
