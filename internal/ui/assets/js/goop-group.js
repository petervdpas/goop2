//
// Group protocol client for site pages.
// Usage:
//
//   <script src="/assets/js/goop-group.js"></script>
//
//   // join a remote group
//   await Goop.group.join(hostPeerId, groupId);
//
//   // send a message to the current group
//   await Goop.group.send({text: "hello"});
//
//   // leave the current group
//   await Goop.group.leave();
//
//   // list hosted groups
//   const groups = await Goop.group.list();
//
//   // subscribe to group events (SSE)
//   Goop.group.subscribe(function(evt) {
//     // evt: {type, group, from, payload}
//   });
//
//   Goop.group.unsubscribe();
//
(() => {
  window.Goop = window.Goop || {};

  let sse = null;

  function post(url, body) {
    return fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then((r) => {
      if (!r.ok) return r.text().then((t) => { throw new Error(t); });
      return r.json();
    });
  }

  window.Goop.group = {
    /** Join a remote group hosted by another peer */
    join(hostPeerId, groupId) {
      return post("/api/groups/join", { host_peer_id: hostPeerId, group_id: groupId });
    },

    /** Send a message/payload to the current group */
    send(payload, groupId) {
      var body = { payload: payload };
      if (groupId) body.group_id = groupId;
      return post("/api/groups/send", body);
    },

    /** Leave the current group */
    leave() {
      return post("/api/groups/leave", {});
    },

    /** Create a hosted group (ID is auto-generated server-side) */
    create(name, appType, maxMembers) {
      return post("/api/groups", {
        name: name,
        app_type: appType || "",
        max_members: maxMembers || 0,
      });
    },

    /** Join a group you host (owner entering own room) */
    joinOwn(groupId) {
      return post("/api/groups/join-own", { group_id: groupId });
    },

    /** Leave a group you host (owner leaving own room) */
    leaveOwn(groupId) {
      return post("/api/groups/leave-own", { group_id: groupId });
    },

    /** Close/delete a hosted group */
    close(groupId) {
      return post("/api/groups/close", { group_id: groupId });
    },

    /** List hosted groups */
    list() {
      return fetch("/api/groups").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /** List subscriptions and active connection */
    subscriptions() {
      return fetch("/api/groups/subscriptions").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /**
     * Subscribe to group events via SSE.
     * callback(evt) where evt has {type, group, from, payload}
     */
    subscribe(callback) {
      if (sse) sse.close();

      sse = new EventSource("/api/groups/events");

      // Listen for all known event types
      var types = ["welcome", "members", "msg", "state", "leave", "close", "error"];
      types.forEach(function(t) {
        sse.addEventListener(t, function(e) {
          try {
            if (callback) callback(JSON.parse(e.data));
          } catch (_) {}
        });
      });

      sse.onerror = function() {};

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
