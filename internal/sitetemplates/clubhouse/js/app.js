(async function () {
  var h = Goop.dom;
  var db = Goop.data;
  var club = db.api("clubhouse");

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

  var ctx = await Goop.peer();
  var myId = ctx.myId;
  var hostId = ctx.hostId;
  var myLabel = ctx.label || (myId ? myId.slice(-6) : "???");
  var isOwner = ctx.isOwner;
  var currentRoom = null;

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

  if (isOwner) btnNewRoom.classList.remove("hidden");

  function shortId(id) { return id ? id.slice(-6) : "???"; }

  function timeStr() {
    var d = new Date();
    var hr = d.getHours(); var mn = d.getMinutes();
    return (hr < 10 ? "0" : "") + hr + ":" + (mn < 10 ? "0" : "") + mn;
  }

  var labelMap = {};
  function displayName(peerId) {
    if (peerId === myId) return "You";
    if (labelMap[peerId]) return labelMap[peerId];
    return shortId(peerId);
  }

  async function loadRooms() {
    try {
      var result = await club("rooms");
      renderRooms(result.rooms || []);
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
        btn.addEventListener("click", function() { if (room) joinRoom(room); });
      });
    });
  }

  btnNewRoom.addEventListener("click", function () {
    document.getElementById("f-name").value = "";
    document.getElementById("f-desc").value = "";
    document.getElementById("f-max").value = "0";
    createOverlay.open();
  });

  document.getElementById("btn-create-cancel").addEventListener("click", function () {
    createOverlay.close();
  });

  document.getElementById("btn-create-save").addEventListener("click", async function () {
    var name = document.getElementById("f-name").value.trim();
    if (!name) return;
    var desc = document.getElementById("f-desc").value.trim();
    var max = parseInt(document.getElementById("f-max").value, 10) || 0;

    try {
      await club("create", { name: name, description: desc, max_members: max });
      createOverlay.close();
      toast("Room created!");
      loadRooms();
    } catch (err) {
      toast({ title: "Error", message: err.message });
    }
  });

  async function joinRoom(room) {
    try {
      var result = await club("join", { room_id: room._id });
      currentRoom = { _id: room._id, name: room.name, description: room.description, group_id: result.group_id };

      Goop.group.subscribe(handleGroupEvent);

      if (!isOwner) {
        await Goop.group.join(hostId, currentRoom.group_id);
      }

      enterChat();
    } catch (err) {
      toast({ title: "Error", message: err.message });
    }
  }

  function enterChat() {
    messagesEl.innerHTML = "";
    chatRoomName.textContent = currentRoom.name;
    chatRoomDesc.textContent = currentRoom.description || "";
    btnCloseRoom.classList.toggle("hidden", !isOwner);
    lobby.classList.add("hidden");
    chatView.classList.remove("hidden");
    appendSystem("You joined the room.");
    fetchMembers();
    startMemberPolling();

    if (!isOwner) {
      Goop.group.send({ type: "presence", label: myLabel }, currentRoom.group_id).catch(function() {});
    }
    msgInput.focus();
  }

  var memberPollTimer = null;
  function startMemberPolling() {
    stopMemberPolling();
    memberPollTimer = setInterval(fetchMembers, 5000);
  }
  function stopMemberPolling() {
    if (memberPollTimer) { clearInterval(memberPollTimer); memberPollTimer = null; }
  }
  function fetchMembers() {
    if (!currentRoom) return;
    club("members", { room_id: currentRoom._id }).then(function(result) {
      renderMembers(result.members || []);
    }).catch(function() {});
  }

  function handleGroupEvent(evt) {
    switch (evt.type) {
      case "welcome":
        if (evt.payload) {
          renderMembersFromPayload(evt.payload);
        }
        break;

      case "members":
        if (evt.payload) {
          renderMembersFromPayload(evt.payload);
        }
        break;

      case "msg":
        if (!evt.payload) break;
        if (evt.from && evt.payload.label) {
          labelMap[evt.from] = evt.payload.label;
        }
        if (evt.payload.type === "presence") break;
        if (evt.payload.type === "chat") {
          var isSelf = evt.from === myId;
          if (isSelf) break;
          appendChat(evt.from, evt.payload.label, evt.payload.text, false);
        }
        break;

      case "close":
        appendSystem("Room was closed by the host.");
        setTimeout(function() { doLeave(); }, 2000);
        break;
    }
  }

  function renderMembers(members) {
    if (!membersListEl) return;
    membersListEl.innerHTML = "";
    members.forEach(function(m) {
      var peerId = m.peer_id || m;
      var li = document.createElement("li");
      li.innerHTML = '<span class="member-dot"></span><span>' + Goop.esc(displayName(peerId)) + '</span>';
      membersListEl.appendChild(li);
    });
  }

  function renderMembersFromPayload(payload) {
    var list = payload.members;
    if (!Array.isArray(list)) return;
    membersListEl.innerHTML = "";
    list.forEach(function(m) {
      var peerId = typeof m === "string" ? m : m.peer_id;
      var li = document.createElement("li");
      li.innerHTML = '<span class="member-dot"></span><span>' + Goop.esc(displayName(peerId)) + '</span>';
      membersListEl.appendChild(li);
    });
  }

  function sendMessage() {
    var text = msgInput.value.trim();
    if (!text || !currentRoom) return;
    msgInput.value = "";

    appendChat(myId, myLabel, text, true);

    Goop.group.send({ type: "chat", text: text, label: myLabel }, currentRoom.group_id).catch(function(err) {
      appendSystem("Send failed: " + err.message);
    });
  }

  btnSend.addEventListener("click", sendMessage);
  msgInput.addEventListener("keydown", function(e) {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  });

  async function doLeave() {
    if (!currentRoom) return;
    stopMemberPolling();
    try { await Goop.group.leave(); } catch (_) {}
    try { await club("leave", { room_id: currentRoom._id }); } catch (_) {}
    Goop.group.unsubscribe();
    currentRoom = null;
    labelMap = {};
    chatView.classList.add("hidden");
    lobby.classList.remove("hidden");
    loadRooms();
  }

  btnBack.addEventListener("click", doLeave);

  btnCloseRoom.addEventListener("click", async function() {
    if (!currentRoom) return;
    var ok = await Goop.ui.confirm("Close this room? All members will be disconnected.");
    if (!ok) return;
    stopMemberPolling();
    try {
      await club("close", { room_id: currentRoom._id });
      toast("Room closed.");
    } catch (err) {
      toast({ title: "Error", message: err.message });
    }
    Goop.group.unsubscribe();
    currentRoom = null;
    labelMap = {};
    chatView.classList.add("hidden");
    lobby.classList.remove("hidden");
    loadRooms();
  });

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

  window.addEventListener("beforeunload", function() {
    if (!currentRoom) return;
    var body = JSON.stringify({ function: "clubhouse", params: { action: "leave", room_id: currentRoom._id } });
    if (navigator.sendBeacon) {
      navigator.sendBeacon("/api/data/lua/call", new Blob([body], { type: "application/json" }));
    }
  });

  Goop.mq.subscribe(function(evt) {
    if (!evt || !evt.msg || !evt.msg.topic) return;
    var parts = evt.msg.topic.split(":");
    if (parts.length === 3 && parts[0] === "group" && parts[2] === "close") {
      if (!currentRoom) {
        loadRooms();
      }
    }
  });

  loadRooms();
})();
