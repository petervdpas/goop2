// Clubhouse app.js — real-time group chat rooms, ORM DSL
(async function () {
  var h = Goop.dom;

  var toast = Goop.ui.toast(document.getElementById("toasts"), {
    toastClass: "gc-toast",
    titleClass: "gc-toast-title",
    messageClass: "gc-toast-message",
    enterClass: "gc-toast-enter",
    exitClass: "gc-toast-exit",
  });

  Goop.ui.dialog(document.getElementById("confirm-dialog"), {
    title: ".gc-dialog-title",
    message: ".gc-dialog-message",
    inputWrap: ".gc-dialog-input-wrap",
    input: ".gc-dialog-input",
    ok: ".gc-dialog-ok",
    cancel: ".gc-dialog-cancel",
    hiddenClass: "hidden",
  });

  var createOverlay = Goop.overlay("create-overlay");

  var esc = Goop.esc;
  var ctx = await Goop.peer();
  var rooms = await Goop.data.orm("rooms");

  // ── DOM refs ──
  var lobby = document.getElementById("lobby");
  var roomsEl = document.getElementById("rooms");
  var btnNewRoom = document.getElementById("btn-new-room");

  var chatView = document.getElementById("chat-view");
  var chatRoomName = document.getElementById("chat-room-name");
  var chatRoomDesc = document.getElementById("chat-room-desc");
  var btnBack = document.getElementById("btn-back");
  var btnCloseRoom = document.getElementById("btn-close-room");
  var membersListEl = document.getElementById("members-list");
  var messagesEl = document.getElementById("messages");
  var msgInput = document.getElementById("msg-input");
  var btnSend = document.getElementById("btn-send");

  var fName = document.getElementById("f-name");
  var fDesc = document.getElementById("f-desc");
  var fMax = document.getElementById("f-max");

  // ── State ──
  var myId = ctx.myId;
  var myLabel = ctx.label || (myId ? myId.slice(-6) : "???");
  var hostPeerId = ctx.hostId;
  var isOwner = ctx.isOwner;
  var currentRoom = null;
  var members = [];
  var labelMap = {};

  if (isOwner) btnNewRoom.classList.remove("hidden");

  function shortId(id) {
    return id ? id.slice(-6) : "???";
  }

  function displayName(peerId) {
    if (peerId === myId) return "You";
    if (labelMap[peerId]) return labelMap[peerId];
    return shortId(peerId);
  }

  function fetchPeerLabels() {
    fetch("/api/peers").then(function (r) {
      if (!r.ok) return;
      return r.json();
    }).then(function (peers) {
      if (!Array.isArray(peers)) return;
      var changed = false;
      peers.forEach(function (p) {
        if (p.ID && p.Content && !labelMap[p.ID]) {
          labelMap[p.ID] = p.Content;
          changed = true;
        }
      });
      if (changed) renderMembers();
    }).catch(function () {});
  }

  // ── Room listing ──
  async function loadRooms() {
    try {
      var rows = await rooms.find({ where: "status = 'open'", limit: 50 });
      renderRooms(rows || []);
    } catch (err) {
      Goop.render(roomsEl, Goop.ui.empty("Could not load rooms.", { class: "empty-msg" }));
    }
  }

  function renderRooms(allRooms) {
    Goop.list(roomsEl, allRooms, "room-card", {
      empty: "No rooms yet." + (isOwner ? " Create one with the button above!" : ""),
      emptyClass: "empty-msg"
    }).then(function() {
      roomsEl.querySelectorAll(".btn-join").forEach(function(btn) {
        var card = btn.closest(".room-card");
        var id = parseInt(card.getAttribute("data-room-id"), 10);
        var room = allRooms.find(function(r) { return r._id === id; });
        btn.addEventListener("click", function() { if (room) enterRoom(room); });
      });
    });
  }

  // ── Create room (owner only) ──
  btnNewRoom.addEventListener("click", function () {
    fName.value = "";
    fDesc.value = "";
    fMax.value = "0";
    createOverlay.open();
  });

  document.getElementById("btn-create-cancel").addEventListener("click", function () {
    createOverlay.close();
  });

  document.getElementById("btn-create-save").addEventListener("click", async function () {
    var name = fName.value.trim();
    if (!name) return;
    var desc = fDesc.value.trim();
    var max = parseInt(fMax.value, 10) || 0;

    try {
      var group = await Goop.group.create(name, "clubhouse", max);
      await rooms.insert({
        name: name,
        description: desc,
        group_id: group.id,
        max_members: max,
        status: "open"
      });
      createOverlay.close();
      toast("Room created!");
      loadRooms();
    } catch (err) {
      toast({ title: "Error", message: err.message });
    }
  });

  // ── Enter room ──
  async function enterRoom(room) {
    currentRoom = room;
    members = [];
    labelMap = {};
    messagesEl.innerHTML = "";
    chatRoomName.textContent = room.name;
    chatRoomDesc.textContent = room.description || "";

    if (isOwner) {
      btnCloseRoom.classList.remove("hidden");
    } else {
      btnCloseRoom.classList.add("hidden");
    }

    lobby.classList.add("hidden");
    chatView.classList.remove("hidden");

    try { await Goop.group.leave(); } catch (_) {}

    Goop.group.subscribe(handleGroupEvent);

    try {
      if (isOwner) {
        await Goop.group.joinOwn(room.group_id);
        labelMap[myId] = myLabel;
        Goop.group.send({ type: "presence", label: myLabel }, room.group_id).catch(function () {});
      } else {
        await Goop.group.join(hostPeerId, room.group_id);
        Goop.group.send({ type: "presence", label: myLabel }).catch(function () {});
      }
      appendSystem("You joined the room.");
      startLabelRefresh();
    } catch (err) {
      appendSystem("Failed to join: " + err.message);
    }

    msgInput.focus();
  }

  // ── Leave room ──
  async function leaveRoom() {
    if (!currentRoom) return;

    try {
      if (isOwner) {
        await Goop.group.leaveOwn(currentRoom.group_id);
      } else {
        await Goop.group.leave();
      }
    } catch (_) {}

    Goop.group.unsubscribe();
    stopLabelRefresh();
    currentRoom = null;
    members = [];

    chatView.classList.add("hidden");
    lobby.classList.remove("hidden");
    loadRooms();
  }

  btnBack.addEventListener("click", leaveRoom);

  // ── Close room (owner only) ──
  btnCloseRoom.addEventListener("click", async function () {
    if (!currentRoom) return;
    var ok = await Goop.ui.confirm("Close this room? All members will be disconnected.");
    if (!ok) return;

    try {
      await Goop.group.close(currentRoom.group_id);
      await rooms.update(currentRoom._id, { status: "closed" });
      toast("Room closed.");
    } catch (err) {
      toast({ title: "Error", message: err.message });
    }

    Goop.group.unsubscribe();
    stopLabelRefresh();
    currentRoom = null;
    members = [];
    chatView.classList.add("hidden");
    lobby.classList.remove("hidden");
    loadRooms();
  });

  // ── Send message ──
  function sendMessage() {
    var text = msgInput.value.trim();
    if (!text || !currentRoom) return;
    msgInput.value = "";

    var payload = { type: "chat", text: text, label: myLabel };

    if (isOwner) {
      Goop.group.send(payload, currentRoom.group_id).catch(function (err) {
        appendSystem("Send failed: " + err.message);
      });
    } else {
      appendChat(myId, myLabel, text, true);
      Goop.group.send(payload).catch(function (err) {
        appendSystem("Send failed: " + err.message);
      });
    }
  }

  btnSend.addEventListener("click", sendMessage);

  msgInput.addEventListener("keydown", function (e) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  });

  // ── SSE event handler ──
  function extractMemberIds(payload) {
    var list = payload && payload.members;
    if (!Array.isArray(list)) return [];
    return list.map(function (m) {
      return typeof m === "string" ? m : m.peer_id;
    });
  }

  function handleGroupEvent(evt) {
    switch (evt.type) {
      case "welcome":
        if (evt.payload) {
          members = extractMemberIds(evt.payload);
          renderMembers();
          fetchPeerLabels();
        }
        break;

      case "members":
        if (evt.payload) {
          var oldCount = members.length;
          members = extractMemberIds(evt.payload);
          renderMembers();
          fetchPeerLabels();
          if (members.length > oldCount) appendSystem("A new member joined.");
          else if (members.length < oldCount) appendSystem("A member left.");
        }
        break;

      case "msg":
        if (!evt.payload) break;
        if (evt.from && evt.payload.label) {
          labelMap[evt.from] = evt.payload.label;
          renderMembers();
        }
        if (evt.payload.type === "presence") break;
        if (evt.payload.type === "chat") {
          var isSelf = evt.from === myId;
          if (!isOwner && isSelf) break;
          appendChat(evt.from, evt.payload.label, evt.payload.text, isSelf);
        }
        break;

      case "close":
        appendSystem("Room was closed by the host.");
        setTimeout(function () {
          Goop.group.unsubscribe();
          stopLabelRefresh();
          currentRoom = null;
          members = [];
          chatView.classList.add("hidden");
          lobby.classList.remove("hidden");
          loadRooms();
        }, 2000);
        break;

      case "leave":
        break;

      case "error":
        appendSystem("Error: " + (evt.payload && evt.payload.message || evt.message || "unknown"));
        break;
    }
  }

  // ── Render helpers ──
  function renderMembers() {
    Goop.list(membersListEl, members, function(peerId) {
      return h("li", {},
        Goop.ui.avatar(peerId, { size: 24, class: "member-avatar" }),
        h("span", { class: "member-dot" }),
        h("span", { class: peerId === myId ? "member-you" : "" }, displayName(peerId))
      );
    });
  }

  function timeStr() {
    var d = new Date();
    var hr = d.getHours(); var mn = d.getMinutes();
    return (hr < 10 ? "0" : "") + hr + ":" + (mn < 10 ? "0" : "") + mn;
  }

  function appendChat(fromId, label, text, isSelf) {
    Goop.partial("message", {
      msgClass: isSelf ? "msg-self" : "msg-other",
      fromLabel: isSelf ? "You" : (label || shortId(fromId)),
      text: text,
      time: timeStr()
    }).then(function(el) {
      messagesEl.appendChild(el);
      messagesEl.scrollTop = messagesEl.scrollHeight;
    });
  }

  function appendSystem(text) {
    var div = document.createElement("div");
    div.className = "msg-system";
    div.textContent = text;
    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
  }

  // ── Periodic peer label refresh ──
  var labelInterval = null;
  function startLabelRefresh() {
    stopLabelRefresh();
    labelInterval = setInterval(fetchPeerLabels, 5000);
  }
  function stopLabelRefresh() {
    if (labelInterval) { clearInterval(labelInterval); labelInterval = null; }
  }

  // ── Clean leave on page/tab close ──
  function doQuickLeave() {
    if (!currentRoom) return;
    var url = isOwner ? "/api/groups/leave-own" : "/api/groups/leave";
    var body = isOwner ? JSON.stringify({ group_id: currentRoom.group_id }) : "{}";
    if (navigator.sendBeacon) {
      navigator.sendBeacon(url, new Blob([body], { type: "application/json" }));
    } else {
      var xhr = new XMLHttpRequest();
      xhr.open("POST", url, false);
      xhr.setRequestHeader("Content-Type", "application/json");
      xhr.send(body);
    }
  }

  window.addEventListener("beforeunload", doQuickLeave);
  window.addEventListener("pagehide", doQuickLeave);

  // ── Init ──
  loadRooms();
})();
