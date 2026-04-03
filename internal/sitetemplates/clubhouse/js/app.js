(async function () {
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
  var chatUnsub = null;

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

  function timeStr() {
    var d = new Date();
    var hr = d.getHours(); var mn = d.getMinutes();
    return (hr < 10 ? "0" : "") + hr + ":" + (mn < 10 ? "0" : "") + mn;
  }

  function displayName(peerId, name) {
    if (peerId === myId) return "You";
    if (name) return name;
    return peerId ? peerId.slice(-6) : "???";
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
      currentRoom = { _id: room._id, name: room.name, description: room.description, group_id: room.group_id };

      if (!isOwner) {
        await Goop.chatroom.join(hostId, currentRoom.group_id);
      }

      chatUnsub = Goop.chatroom.subscribe(function(groupId, action, data) {
        if (groupId !== currentRoom.group_id) return;
        handleChatEvent(action, data);
      });

      enterChat();
    } catch (err) {
      toast({ title: "Error", message: err.message });
      currentRoom = null;
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

    Goop.chatroom.state(currentRoom.group_id).then(function(result) {
      if (result.room && result.room.members) renderMembers(result.room.members);
      if (result.messages) {
        result.messages.forEach(function(m) {
          appendChat(m.from, m.from_name, m.text, m.from === myId);
        });
      }
    }).catch(function() {});

    msgInput.focus();
  }

  function handleChatEvent(action, data) {
    switch (action) {
      case "msg":
        if (data.message) {
          var m = data.message;
          if (m.from === myId) break;
          appendChat(m.from, m.from_name, m.text, false);
        }
        break;

      case "history":
        if (data.messages) {
          data.messages.forEach(function(m) {
            appendChat(m.from, m.from_name, m.text, m.from === myId);
          });
        }
        break;

      case "members":
        if (data.members) renderMembers(data.members);
        break;
    }
  }

  function renderMembers(members) {
    if (!membersListEl) return;
    membersListEl.innerHTML = "";
    members.forEach(function(m) {
      var li = document.createElement("li");
      li.innerHTML = '<span class="member-dot"></span><span>' + Goop.esc(displayName(m.peer_id, m.name)) + '</span>';
      membersListEl.appendChild(li);
    });
  }

  function sendMessage() {
    var text = msgInput.value.trim();
    if (!text || !currentRoom) return;
    msgInput.value = "";

    appendChat(myId, myLabel, text, true);

    Goop.chatroom.send(currentRoom.group_id, text).catch(function(err) {
      appendSystem("Send failed: " + err.message);
    });
  }

  btnSend.addEventListener("click", sendMessage);
  msgInput.addEventListener("keydown", function(e) {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  });

  async function doLeave() {
    if (!currentRoom) return;
    if (chatUnsub) { chatUnsub(); chatUnsub = null; }
    try { await Goop.chatroom.leave(currentRoom.group_id); } catch (_) {}
    currentRoom = null;
    chatView.classList.add("hidden");
    lobby.classList.remove("hidden");
    loadRooms();
  }

  btnBack.addEventListener("click", doLeave);

  btnCloseRoom.addEventListener("click", async function() {
    if (!currentRoom) return;
    var ok = await Goop.ui.confirm("Close this room? All members will be disconnected.");
    if (!ok) return;
    try {
      await club("close", { room_id: currentRoom._id });
      toast("Room closed.");
    } catch (err) {
      toast({ title: "Error", message: err.message });
    }
    if (chatUnsub) { chatUnsub(); chatUnsub = null; }
    currentRoom = null;
    chatView.classList.add("hidden");
    lobby.classList.remove("hidden");
    loadRooms();
  });

  function appendChat(fromId, label, text, isSelf) {
    Goop.partial("message", {
      msgClass: isSelf ? "msg-self" : "msg-other",
      fromLabel: isSelf ? "You" : (label || (fromId ? fromId.slice(-6) : "???")),
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
    if (navigator.sendBeacon) {
      navigator.sendBeacon("/api/chat/rooms/leave", new Blob([JSON.stringify({ group_id: currentRoom.group_id })], { type: "application/json" }));
    }
  });

  loadRooms();
})();
