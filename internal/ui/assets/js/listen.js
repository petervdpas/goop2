// Listen group — host & listener UI logic
(function () {
  if (!window.Goop) window.Goop = {};

  var api = {
    create: function (name) {
      return fetch("/api/listen/create", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: name }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    close: function () {
      return fetch("/api/listen/close", { method: "POST" }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    load: function (filePath) {
      return fetch("/api/listen/load", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ file_path: filePath }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    loadQueue: function (filePaths) {
      return fetch("/api/listen/load", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ file_paths: filePaths }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    addToQueue: function (filePaths) {
      return fetch("/api/listen/queue/add", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ file_paths: filePaths }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    control: function (action, position, index) {
      return fetch("/api/listen/control", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: action, position: position || 0, index: index || 0 }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    join: function (hostPeerId, groupId) {
      return fetch("/api/listen/join", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host_peer_id: hostPeerId, group_id: groupId }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    leave: function () {
      return fetch("/api/listen/leave", { method: "POST" }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    state: function () {
      return fetch("/api/listen/state").then(function (r) { return r.json(); });
    },

    // subscribe(callback) — SSE subscription for peer sites.
    // callback receives the group object (or null) on every state change.
    // Returns { close: function } to stop the subscription.
    subscribe: function (callback) {
      var es = new EventSource("/api/listen/events");
      es.addEventListener("state", function (e) {
        try {
          var data = JSON.parse(e.data);
          callback(data.group);
        } catch (err) {
          console.error("LISTEN subscribe parse error:", err);
        }
      });
      es.addEventListener("connected", function () {
        // Fetch initial state on connect
        api.state().then(function (data) {
          callback(data.group);
        });
      });
      return {
        close: function () {
          es.close();
        },
      };
    },

    // streamURL() — returns the audio stream URL for use in <audio> elements.
    streamURL: function () {
      return "/api/listen/stream";
    },
  };

  Goop.listen = api;
})();
