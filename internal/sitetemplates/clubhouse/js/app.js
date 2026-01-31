// Clubhouse app.js — real-time group chat rooms
(async function () {
  var db = Goop.data;

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

  var createOverlay = document.getElementById("create-overlay");
  var fName = document.getElementById("f-name");
  var fDesc = document.getElementById("f-desc");
  var fMax = document.getElementById("f-max");
  var btnCreateCancel = document.getElementById("btn-create-cancel");
  var btnCreateSave = document.getElementById("btn-create-save");

  // ── State ──
  var isOwner = false;
  var myId = "";
  var myLabel = "";
  var hostPeerId = "";
  var currentRoom = null;     // room row from DB
  var members = [];           // current member peer IDs
  var labelMap = {};          // peerId → display label

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  function shortId(id) {
    return id ? id.slice(-6) : "???";
  }

  function displayName(peerId) {
    if (peerId === myId) return "You";
    if (labelMap[peerId]) return labelMap[peerId];
    return shortId(peerId);
  }

  // ── Owner detection ──
  try {
    var me = await Goop.identity.get();
    myId = me.id;
    myLabel = me.label || shortId(myId);
    var match = window.location.pathname.match(/\/p\/([^/]+)/);
    if (match) {
      hostPeerId = match[1];
      if (match[1] === myId) {
        isOwner = true;
        btnNewRoom.classList.remove("hidden");
      }
    }
  } catch (_) {}

  // ── Room listing ──
  async function loadRooms() {
    try {
      var rows = await db.query("rooms", { where: "status = 'open'", limit: 50 });
      renderRooms(rows || []);
    } catch (err) {
      roomsEl.innerHTML = '<div class="empty-msg"><p>Could not load rooms.</p></div>';
    }
  }

  function renderRooms(rooms) {
    if (rooms.length === 0) {
      roomsEl.innerHTML = '<div class="empty-msg"><div class="empty-icon">&#128172;</div><p>No rooms yet.</p>' +
        (isOwner ? '<p>Create one with the button above!</p>' : '') + '</div>';
      return;
    }

    roomsEl.innerHTML = rooms.map(function (r) {
      var html = '<div class="room-card" data-room-id="' + r._id + '">';
      html += '<h3 class="room-card-name">' + esc(r.name) + '</h3>';
      html += '<p class="room-card-desc">' + esc(r.description || "No description") + '</p>';
      html += '<div class="room-card-footer">';
      html += '<span class="room-card-status"><span class="status-dot"></span> ' + esc(r.status) + '</span>';
      html += '<button class="btn-join" data-room-id="' + r._id + '">Join</button>';
      html += '</div></div>';
      return html;
    }).join("");

    roomsEl.querySelectorAll(".btn-join").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var id = parseInt(btn.getAttribute("data-room-id"), 10);
        var room = rooms.find(function (r) { return r._id === id; });
        if (room) enterRoom(room);
      });
    });
  }

  // ── Create room (owner only) ──
  btnNewRoom.addEventListener("click", function () {
    fName.value = "";
    fDesc.value = "";
    fMax.value = "0";
    createOverlay.classList.remove("hidden");
    fName.focus();
  });

  btnCreateCancel.addEventListener("click", function () {
    createOverlay.classList.add("hidden");
  });

  createOverlay.addEventListener("mousedown", function (e) {
    if (e.target === createOverlay) createOverlay.classList.add("hidden");
  });

  btnCreateSave.addEventListener("click", async function () {
    var name = fName.value.trim();
    if (!name) return;
    var desc = fDesc.value.trim();
    var max = parseInt(fMax.value, 10) || 0;

    try {
      var group = await Goop.group.create(name, "clubhouse", max);
      await db.insert("rooms", {
        name: name,
        description: desc,
        group_id: group.id,
        max_members: max,
        status: "open"
      });
      createOverlay.classList.add("hidden");
      Goop.ui.toast("Room created!");
      loadRooms();
    } catch (err) {
      Goop.ui.toast({ title: "Error", message: err.message });
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

    // Subscribe to SSE first
    Goop.group.subscribe(handleGroupEvent);

    try {
      if (isOwner) {
        await Goop.group.joinOwn(room.group_id);
        // Owner knows own label already
        labelMap[myId] = myLabel;
      } else {
        await Goop.group.join(hostPeerId, room.group_id);
        // Announce our label so other members see a friendly name
        Goop.group.send({ type: "presence", label: myLabel }).catch(function () {});
      }
      appendSystem("You joined the room.");
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
      await db.update("rooms", currentRoom._id, { status: "closed" });
      Goop.ui.toast("Room closed.");
    } catch (err) {
      Goop.ui.toast({ title: "Error", message: err.message });
    }

    Goop.group.unsubscribe();
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
      // Owner: message comes back via SSE, displayed by event handler
      Goop.group.send(payload, currentRoom.group_id).catch(function (err) {
        appendSystem("Send failed: " + err.message);
      });
    } else {
      // Visitor: message NOT echoed back — append locally first
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
  // Server sends: {type, group, from, payload}
  // welcome payload: {group_name, members: [{peer_id, joined_at}]}
  // members payload: {members: [{peer_id, joined_at}]}
  // msg payload: the raw message object from the sender

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
        }
        break;

      case "members":
        if (evt.payload) {
          var oldCount = members.length;
          members = extractMemberIds(evt.payload);
          renderMembers();
          if (members.length > oldCount) {
            appendSystem("A new member joined.");
          } else if (members.length < oldCount) {
            appendSystem("A member left.");
          }
        }
        break;

      case "msg":
        if (!evt.payload) break;

        // Track labels from any message that carries one
        if (evt.from && evt.payload.label) {
          labelMap[evt.from] = evt.payload.label;
          renderMembers();
        }

        if (evt.payload.type === "presence") {
          // Presence is label-only, no visible message
          break;
        }

        if (evt.payload.type === "chat") {
          var isSelf = evt.from === myId;
          // Visitors already appended their own messages locally
          if (!isOwner && isSelf) break;
          appendChat(evt.from, evt.payload.label, evt.payload.text, isSelf);
        }
        break;

      case "close":
        appendSystem("Room was closed by the host.");
        setTimeout(function () {
          Goop.group.unsubscribe();
          currentRoom = null;
          members = [];
          chatView.classList.add("hidden");
          lobby.classList.remove("hidden");
          loadRooms();
        }, 2000);
        break;

      case "leave":
        // A member left — members event will follow
        break;

      case "error":
        appendSystem("Error: " + (evt.payload && evt.payload.message || evt.message || "unknown"));
        break;
    }
  }

  // ── Render helpers ──
  function renderMembers() {
    membersListEl.innerHTML = members.map(function (peerId) {
      var name = displayName(peerId);
      var cls = peerId === myId ? ' class="member-you"' : '';
      return '<li><span class="member-dot"></span><span' + cls + '>' + esc(name) + '</span></li>';
    }).join("");
  }

  function timeStr() {
    var d = new Date();
    var h = d.getHours(); var m = d.getMinutes();
    return (h < 10 ? "0" : "") + h + ":" + (m < 10 ? "0" : "") + m;
  }

  function appendChat(fromId, label, text, isSelf) {
    var div = document.createElement("div");
    div.className = "msg " + (isSelf ? "msg-self" : "msg-other");

    var labelText = isSelf ? "You" : esc(label || shortId(fromId));
    div.innerHTML = '<div class="msg-label">' + labelText + '</div>' +
                    '<div class="msg-text">' + esc(text) + '</div>' +
                    '<div class="msg-time">' + timeStr() + '</div>';

    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
  }

  function appendSystem(text) {
    var div = document.createElement("div");
    div.className = "msg-system";
    div.textContent = text;
    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
  }

  // ── Init ──
  loadRooms();
})();
