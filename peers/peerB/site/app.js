// app.js — peerB demo page
// Showcases: goop-identity, goop-peers, goop-chat, goop-ui, goop-site

(async function () {
  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  function shortId(id) {
    return id && id.length > 10 ? id.substring(0, 10) + "..." : id;
  }

  // ── Identity ──
  var identityEl = document.querySelector("#identity p");
  try {
    var me = await Goop.identity.get();
    identityEl.innerHTML =
      '<span class="label">ID:</span> <code>' + esc(shortId(me.id)) + "</code>" +
      ' &nbsp; <span class="label">Label:</span> <code>' + esc(me.label) + "</code>";
  } catch (err) {
    identityEl.textContent = "Failed to load identity";
  }

  // ── Peers (live) ──
  var peerListEl = document.getElementById("peer-list");
  var peerCountEl = document.getElementById("peer-count");

  function renderPeers(peers) {
    peerCountEl.textContent = "(" + peers.length + ")";
    if (!peers || peers.length === 0) {
      peerListEl.innerHTML = '<li class="muted">No peers online</li>';
      return;
    }
    peerListEl.innerHTML = peers
      .map(function (p) {
        var label = p.Content ? " &mdash; " + esc(p.Content) : "";
        return "<li><code>" + esc(shortId(p.ID)) + "</code>" + label + "</li>";
      })
      .join("");
  }

  Goop.peers.subscribe({
    onSnapshot: function (peers) {
      renderPeers(peers);
    },
    onUpdate: function () {
      Goop.peers.list().then(renderPeers);
    },
    onRemove: function () {
      Goop.peers.list().then(renderPeers);
    },
  });

  // initial fetch
  try {
    renderPeers(await Goop.peers.list());
  } catch (_) {}

  // ── Chat ──
  var chatBox = document.getElementById("chat-messages");
  var chatForm = document.getElementById("chat-form");
  var chatInput = document.getElementById("chat-input");
  var selfId = null;

  try { selfId = await Goop.identity.id(); } catch (_) {}

  async function loadChat() {
    try {
      var msgs = await Goop.chat.broadcasts();
      if (!msgs || msgs.length === 0) {
        chatBox.innerHTML = '<p class="muted">No messages yet. Say hello!</p>';
        return;
      }
      chatBox.innerHTML = msgs
        .map(function (m) {
          var who = m.from === selfId ? "me" : shortId(m.from);
          var cls = m.from === selfId ? "msg-out" : "msg-in";
          var time = new Date(m.timestamp).toLocaleTimeString();
          return (
            '<div class="msg ' + cls + '">' +
            '<span class="msg-who">' + esc(who) + "</span> " +
            '<span class="msg-text">' + esc(m.content) + "</span> " +
            '<span class="msg-time">' + esc(time) + "</span>" +
            "</div>"
          );
        })
        .join("");
      chatBox.scrollTop = chatBox.scrollHeight;
    } catch (err) {
      chatBox.innerHTML = '<p class="error">Failed to load messages</p>';
    }
  }

  chatForm.addEventListener("submit", async function (e) {
    e.preventDefault();
    var text = chatInput.value.trim();
    if (!text) return;
    try {
      await Goop.chat.broadcast(text);
      chatInput.value = "";
      Goop.ui.toast("Message sent");
      loadChat();
    } catch (err) {
      Goop.ui.toast({ title: "Error", message: err.message, duration: 5000 });
    }
  });

  // live updates
  Goop.chat.subscribe(function (msg) {
    var type = msg.type || "direct";
    if (type === "broadcast") loadChat();
  });

  loadChat();

  // ── Site Files ──
  var fileListEl = document.getElementById("file-list");
  try {
    var files = await Goop.site.files();
    if (!files || files.length === 0) {
      fileListEl.innerHTML = '<li class="muted">No files</li>';
    } else {
      fileListEl.innerHTML = files
        .map(function (f) {
          var indent = f.Depth ? "padding-left:" + f.Depth * 1.2 + "rem" : "";
          var icon = f.IsDir ? "dir" : "file";
          return (
            '<li style="' + indent + '">' +
            '<span class="file-icon ' + icon + '"></span>' +
            esc(f.Path) +
            "</li>"
          );
        })
        .join("");
    }
  } catch (err) {
    fileListEl.innerHTML = '<li class="error">Failed to load files</li>';
  }
})();
