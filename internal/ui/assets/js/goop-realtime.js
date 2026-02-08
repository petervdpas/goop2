//
// Real-time channel API for peer-to-peer communication.
// Wraps the /api/realtime/* endpoints + SSE for receiving.
//
// Usage:
//   <script src="/assets/js/goop-realtime.js"></script>
//
//   // Connect to a peer
//   const channel = await Goop.realtime.connect(peerId);
//   channel.send({ type: "move", from: "e2", to: "e4" });
//   channel.onMessage(function(msg) { console.log(msg); });
//   channel.close();
//
//   // Accept an incoming channel
//   Goop.realtime.onIncoming(function(info) {
//     const channel = await Goop.realtime.accept(info.channelId, info.hostPeerId);
//     channel.onMessage(function(msg) { ... });
//   });
//
//   // List active channels
//   const channels = await Goop.realtime.channels();
//
(() => {
  window.Goop = window.Goop || {};

  var apiBase = "/api/realtime";
  var messageHandlers = {};    // channelID -> [callbacks]
  var globalHandlers = [];     // callbacks for all channels
  var incomingHandlers = [];   // callbacks for incoming channel invites
  var eventSource = null;
  var connected = false;

  async function request(url, opts) {
    var res = await fetch(url, opts);
    if (!res.ok) {
      var text = await res.text();
      throw new Error(text || res.statusText);
    }
    return res.json();
  }

  function post(path, body) {
    return request(apiBase + path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    });
  }

  // ── SSE connection ──────────────────────────────────────────────────────────

  function ensureSSE() {
    if (eventSource) return;

    eventSource = new EventSource(apiBase + "/events");

    eventSource.addEventListener("connected", function() {
      connected = true;
    });

    eventSource.addEventListener("message", function(e) {
      try {
        var env = JSON.parse(e.data);
        var channelId = env.channel;
        var payload = env.payload;

        // Notify channel-specific handlers
        var cbs = messageHandlers[channelId];
        if (cbs) {
          for (var i = 0; i < cbs.length; i++) {
            try { cbs[i](payload, env); } catch(err) { console.error("realtime handler error:", err); }
          }
        }

        // Notify global handlers
        for (var j = 0; j < globalHandlers.length; j++) {
          try { globalHandlers[j](payload, env); } catch(err) { console.error("realtime handler error:", err); }
        }
      } catch(err) {
        console.error("realtime: failed to parse message:", err);
      }
    });

    eventSource.onerror = function() {
      connected = false;
      // Auto-reconnect is handled by EventSource
    };
  }

  // ── Channel wrapper ─────────────────────────────────────────────────────────

  function wrapChannel(info) {
    var channelId = info.id;

    return {
      id: channelId,
      remotePeer: info.remote_peer,
      role: info.role,

      send: function(payload) {
        return post("/send", { channel_id: channelId, payload: payload });
      },

      onMessage: function(callback) {
        if (!messageHandlers[channelId]) {
          messageHandlers[channelId] = [];
        }
        messageHandlers[channelId].push(callback);
      },

      offMessage: function(callback) {
        var cbs = messageHandlers[channelId];
        if (!cbs) return;
        var idx = cbs.indexOf(callback);
        if (idx >= 0) cbs.splice(idx, 1);
      },

      close: function() {
        delete messageHandlers[channelId];
        return post("/close", { channel_id: channelId });
      }
    };
  }

  // ── Listen for incoming channel invites via group events ────────────────────

  function listenForInvites() {
    var grpSource = new EventSource("/api/groups/events");

    grpSource.addEventListener("welcome", function(e) {
      try {
        var evt = JSON.parse(e.data);
        // Check if this is a realtime channel
        if (evt.payload && evt.payload.app_type === "realtime") {
          var info = {
            channelId: evt.group,
            hostPeerId: evt.from || (evt.payload && evt.payload.host_peer_id) || ""
          };
          for (var i = 0; i < incomingHandlers.length; i++) {
            try { incomingHandlers[i](info); } catch(err) { console.error("incoming handler error:", err); }
          }
        }
      } catch(err) {
        // ignore parse errors
      }
    });
  }

  // ── Public API ──────────────────────────────────────────────────────────────

  Goop.realtime = {
    // Connect to a peer, creating a new realtime channel
    connect: async function(peerId) {
      ensureSSE();
      var info = await post("/connect", { peer_id: peerId });
      return wrapChannel(info);
    },

    // Accept an incoming channel invitation
    accept: async function(channelId, hostPeerId) {
      ensureSSE();
      var info = await post("/accept", { channel_id: channelId, host_peer_id: hostPeerId });
      return wrapChannel(info);
    },

    // List active channels
    channels: function() {
      return request(apiBase + "/channels");
    },

    // Register handler for incoming channel invites
    onIncoming: function(callback) {
      if (incomingHandlers.length === 0) {
        listenForInvites();
      }
      incomingHandlers.push(callback);
    },

    // Register handler for all messages on all channels
    onMessage: function(callback) {
      ensureSSE();
      globalHandlers.push(callback);
    },

    // Remove a global message handler
    offMessage: function(callback) {
      var idx = globalHandlers.indexOf(callback);
      if (idx >= 0) globalHandlers.splice(idx, 1);
    },

    // Check if SSE is connected
    isConnected: function() {
      return connected;
    }
  };
})();
