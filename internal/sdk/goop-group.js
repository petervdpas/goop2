//
// Group protocol client for site pages.
// Usage:
//
//   <script src="/sdk/goop-group.js"></script>
//
//   // join a remote group (connects MQ stream to host)
//   await Goop.group.join(hostPeerId, groupId);
//
//   // send a message to the current group
//   await Goop.group.send({text: "hello"});
//
//   // leave the current group
//   await Goop.group.leave();
//
//   // list subscriptions (groups you have joined as a member)
//   const data = await Goop.group.subscriptions();
//   // data.subscriptions: [{group_id, group_name, host_peer_id, host_name,
//   //                        group_type, role, member_count, host_reachable}]
//   // data.active_groups: [{group_id}]
//
//   // subscribe to group events (SSE)
//   Goop.group.subscribe(function(evt) {
//     // evt: { type, group, from, payload }
//     // type: "welcome", "members", "msg", "state", "leave", "close", "error", "invite"
//   });
//
//   Goop.group.unsubscribe();
//
// Group management (create, close, add/remove members) is done server-side
// through Lua via goop.group.create/close/add/remove/send/members/list.
// Templates call Lua functions for management; this SDK is for MQ messaging.
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
    /** Join a remote group hosted by another peer (connects MQ stream) */
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

    /** List subscriptions and active connections */
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

      var types = ["welcome", "members", "msg", "state", "leave", "close", "error", "invite"];
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
