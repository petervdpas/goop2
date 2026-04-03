//
// Chat room SDK for site pages.
// Usage:
//
//   <script src="/sdk/goop-chatroom.js"></script>
//
//   // create a chat room (host only)
//   const room = await Goop.chatroom.create("Room Name", "description", 10);
//
//   // join a remote room
//   await Goop.chatroom.join(hostPeerId, groupId);
//
//   // send a message
//   await Goop.chatroom.send(groupId, "hello");
//
//   // get room state (members + recent messages)
//   const state = await Goop.chatroom.state(groupId);
//
//   // close a room (host only)
//   await Goop.chatroom.close(groupId);
//
//   // leave a room
//   await Goop.chatroom.leave(groupId);
//
//   // subscribe to chat room events via MQ
//   const unsub = Goop.chatroom.subscribe(function(groupId, action, data) {
//     // action: "msg", "history", "members"
//   });
//
//   unsub(); // stop listening
//
(() => {
  window.Goop = window.Goop || {};

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

  window.Goop.chatroom = {
    create(name, description, maxMembers, context) {
      return post("/api/chat/rooms/create", {
        name: name,
        description: description || "",
        context: context || "",
        max_members: maxMembers || 0,
      });
    },

    close(groupId) {
      return post("/api/chat/rooms/close", { group_id: groupId });
    },

    join(hostPeerId, groupId) {
      return post("/api/chat/rooms/join", {
        host_peer_id: hostPeerId,
        group_id: groupId,
      });
    },

    leave(groupId) {
      return post("/api/chat/rooms/leave", { group_id: groupId });
    },

    send(groupId, text) {
      return post("/api/chat/rooms/send", {
        group_id: groupId,
        text: text,
      });
    },

    state(groupId) {
      return fetch("/api/chat/rooms/state?group_id=" + encodeURIComponent(groupId))
        .then((r) => {
          if (!r.ok) throw new Error(r.statusText);
          return r.json();
        });
    },

    subscribe(callback) {
      if (!window.Goop || !window.Goop.mq) return function() {};
      return Goop.mq.onChatRoom(function(from, topic, payload, ack) {
        ack();
        if (!payload) return;
        var prefix = Goop.mq.TOPICS.CHATROOM_PREFIX;
        var rest = topic.slice(prefix.length);
        var idx = rest.lastIndexOf(":");
        if (idx < 0) return;
        var groupId = rest.substring(0, idx);
        var action = rest.substring(idx + 1);
        callback(groupId, action, payload);
      });
    },
  };
})();
