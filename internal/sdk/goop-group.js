//
// Group protocol client for site pages.
// Usage:
//
//   <script src="/sdk/goop-group.js"></script>
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
//   // Each group object:
//   // {
//   //   id:           string   — group ID
//   //   name:         string   — display name
//   //   app_type:     string   — e.g. "files", "listen", "realtime", or ""
//   //   max_members:  number   — 0 means unlimited
//   //   volatile:     bool     — group is destroyed when all members leave
//   //   host_joined:  bool     — host has joined their own group
//   //   host_in_group: bool    — host is currently an active member
//   //   created_at:   string   — creation timestamp
//   //   member_count: number   — current number of members
//   //   members: [             — current member list
//   //     { peer_id: string, joined_at: number, name: string }
//   //   ]
//   // }
//
//   // list subscriptions (groups you have joined as a member)
//   const data = await Goop.group.subscriptions();
//   // data.subscriptions: [{group_id, group_name, host_peer_id, host_name,
//   //                        app_type, role, member_count, host_reachable}]
//   // data.active_groups: [{group_id}]  — currently connected
//
//   // subscribe to group events (SSE)
//   Goop.group.subscribe(function(evt) {
//     // evt: { type, group, from, payload }
//     // type values: "welcome", "members", "msg", "state", "leave", "close", "error", "invite"
//     //
//     // "welcome"  — you joined; payload: {group_name, members, app_type, ...}
//     // "members"  — membership changed; payload: {members: [{peer_id, name}]}
//     // "msg"      — message from a member; payload: any (app-defined)
//     // "state"    — shared state update; payload: any
//     // "leave"    — a member left; from: peer_id
//     // "close"    — group was closed by host
//     // "invite"   — you received an invite to a new group
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

    /** List hosted groups. Returns array of group objects (see file header for fields). */
    list() {
      return fetch("/api/groups").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /** List subscriptions and active connections (see file header for fields). */
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
