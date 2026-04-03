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
  var myLabel = ctx.label || (myId ? myId.slice(-6) : "???");
  var isOwner = ctx.isOwner;
  var currentRoom = null;
  var pollTimer = null;

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
      currentRoom = { _id: room._id, name: result.name, description: result.description, group_id: result.group_id };
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
    startPolling();
    msgInput.focus();
  }

  function startPolling() {
    stopPolling();
    pollTimer = setInterval(async function() {
      if (!currentRoom) return;
      try {
        var result = await club("members", { room_id: currentRoom._id });
        renderMembers(result.members || []);
      } catch (_) {}
    }, 5000);
    club("members", { room_id: currentRoom._id }).then(function(result) {
      renderMembers(result.members || []);
    }).catch(function() {});
  }

  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }

  function renderMembers(members) {
    if (!membersListEl) return;
    membersListEl.innerHTML = "";
    members.forEach(function(m) {
      var li = document.createElement("li");
      var name = m.peer_id === myId ? "You" : shortId(m.peer_id);
      li.innerHTML = '<span class="member-dot"></span><span>' + Goop.esc(name) + '</span>';
      membersListEl.appendChild(li);
    });
  }

  function sendMessage() {
    var text = msgInput.value.trim();
    if (!text || !currentRoom) return;
    msgInput.value = "";

    appendChat(myId, myLabel, text, true);

    club("send_message", { room_id: currentRoom._id, text: text }).catch(function(err) {
      appendSystem("Send failed: " + err.message);
    });
  }

  btnSend.addEventListener("click", sendMessage);
  msgInput.addEventListener("keydown", function(e) {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  });

  async function leaveRoom() {
    if (!currentRoom) return;
    stopPolling();
    try { await club("leave", { room_id: currentRoom._id }); } catch (_) {}
    currentRoom = null;
    chatView.classList.add("hidden");
    lobby.classList.remove("hidden");
    loadRooms();
  }

  btnBack.addEventListener("click", leaveRoom);

  btnCloseRoom.addEventListener("click", async function() {
    if (!currentRoom) return;
    var ok = await Goop.ui.confirm("Close this room? All members will be disconnected.");
    if (!ok) return;
    stopPolling();
    try {
      await club("close", { room_id: currentRoom._id });
      toast("Room closed.");
    } catch (err) {
      toast({ title: "Error", message: err.message });
    }
    currentRoom = null;
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
    if (currentRoom) {
      var url = "/api/data/lua/call";
      var body = JSON.stringify({ function: "clubhouse", params: { action: "leave", room_id: currentRoom._id } });
      if (navigator.sendBeacon) {
        navigator.sendBeacon(url, new Blob([body], { type: "application/json" }));
      }
    }
  });

  loadRooms();
})();
