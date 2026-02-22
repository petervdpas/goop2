// Blog app.js
(async function () {
  var db = Goop.data;
  var postsEl = document.getElementById("posts");
  var btnNew = document.getElementById("btn-new");
  var overlay = document.getElementById("editor-overlay");
  var isOwner = false;
  var isCoAuthor = false;
  var myId = null;
  var editingId = null; // null = new post, number = editing existing

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  // ── Owner / co-author detection ──
  // Compare our peer ID with the peer ID in the URL path /p/{id}/
  try {
    myId = await Goop.identity.id();
    var match = window.location.pathname.match(/\/p\/([^/]+)/);
    var ownerPeerId = match ? match[1] : null;
    if (ownerPeerId && ownerPeerId === myId) {
      isOwner = true;
    } else if (ownerPeerId && Goop.group) {
      // Check if we have a "template" group subscription from this host
      var subs = await Goop.group.subscriptions();
      var list = (subs && subs.subscriptions) || [];
      isCoAuthor = list.some(function (s) {
        return s.host_peer_id === ownerPeerId && s.app_type === "template";
      });
    }
  } catch (_) {}

  if (isOwner || isCoAuthor) {
    btnNew.classList.remove("hidden");
  }

  // ── Seed sample posts on first run ──
  async function seed() {
    var tables = await db.tables();
    if (tables && tables.length > 0) return;

    await db.createTable("posts", [
      { name: "title", type: "TEXT", not_null: true },
      { name: "body", type: "TEXT", not_null: true },
      { name: "author_name", type: "TEXT", default: "" },
      { name: "slug", type: "TEXT" },
      { name: "published", type: "INTEGER", default: "1" },
    ]);

    var myLabel = "";
    try { myLabel = await Goop.identity.label(); } catch (_) {}

    await db.insert("posts", {
      title: "Hello, World!",
      body: "Welcome to my blog. This is my first post on the ephemeral web.\n\nI'm running a peer-to-peer site using Goop². Everything here is local-first and distributed — no central servers involved.\n\nFeel free to look around!",
      slug: "hello-world",
      author_name: myLabel,
    });

    await db.insert("posts", {
      title: "How This Works",
      body: "Each peer runs their own site. You're reading this through the p2p network right now.\n\nI write posts from my local editor, and they get served to anyone who connects. No accounts, no passwords, no cloud — just peers talking to peers.",
      slug: "how-this-works",
      author_name: myLabel,
    });
  }

  // ── Load & render ──
  async function loadPosts() {
    try {
      var rows = await db.query("posts", {
        where: "published = 1",
        limit: 50,
      });
      renderPosts(rows || []);
    } catch (err) {
      postsEl.innerHTML = '<div class="empty-msg"><p>Could not load posts.</p><p class="loading">' + esc(err.message) + "</p></div>";
    }
  }

  function renderPosts(posts) {
    if (posts.length === 0) {
      postsEl.innerHTML = '<div class="empty-msg"><p>No posts yet.</p>' +
        ((isOwner || isCoAuthor) ? '<p class="loading">Click "+ New Post" to write your first one.</p>' : '') +
        "</div>";
      return;
    }

    // newest first
    posts.sort(function (a, b) { return b._id - a._id; });

    postsEl.innerHTML = posts.map(function (p) {
      var date = p._created_at ? new Date(String(p._created_at).replace(" ", "T") + "Z").toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" }) : "";
      var html = '<article class="post">';
      html += '<h2 class="post-title">' + esc(p.title) + "</h2>";
      html += '<div class="post-meta">' + esc(date) + "</div>";
      if (p.author_name) {
        html += '<div class="post-byline">by ' + esc(p.author_name) + "</div>";
      }
      html += '<div class="post-body">' + esc(p.body) + "</div>";
      var canEdit = isOwner || (isCoAuthor && p._owner === myId);
      if (canEdit) {
        html += '<div class="post-actions">';
        html += '<button data-action="edit" data-id="' + p._id + '">Edit</button>';
        html += '<button data-action="delete" data-id="' + p._id + '">Delete</button>';
        html += "</div>";
      }
      html += "</article>";
      return html;
    }).join("");

    if (isOwner || isCoAuthor) wireActions();
  }

  function wireActions() {
    postsEl.querySelectorAll("[data-action=edit]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var id = parseInt(btn.getAttribute("data-id"), 10);
        openEditor(id);
      });
    });
    postsEl.querySelectorAll("[data-action=delete]").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        var id = parseInt(btn.getAttribute("data-id"), 10);
        var ok = true;
        if (Goop.ui) ok = await Goop.ui.confirm("Delete this post?");
        if (!ok) return;
        await db.remove("posts", id);
        loadPosts();
      });
    });
  }

  // ── Editor ──
  async function openEditor(id) {
    editingId = id || null;
    document.getElementById("f-title").value = "";
    document.getElementById("f-body").value = "";
    document.getElementById("editor-heading").textContent = id ? "Edit Post" : "New Post";
    document.getElementById("btn-save").textContent = id ? "Update" : "Publish";

    if (id) {
      try {
        var rows = await db.query("posts", { where: "_id = ?", args: [id], limit: 1 });
        if (rows && rows.length > 0) {
          document.getElementById("f-title").value = rows[0].title;
          document.getElementById("f-body").value = rows[0].body;
        }
      } catch (_) {}
    }
    overlay.classList.remove("hidden");
    document.getElementById("f-title").focus();
  }

  btnNew.addEventListener("click", function () { openEditor(null); });

  document.getElementById("btn-cancel").addEventListener("click", function () {
    overlay.classList.add("hidden");
  });

  overlay.addEventListener("mousedown", function (e) {
    if (e.target === overlay) overlay.classList.add("hidden");
  });

  document.getElementById("btn-save").addEventListener("click", async function () {
    var title = document.getElementById("f-title").value.trim();
    var body = document.getElementById("f-body").value.trim();
    if (!title || !body) return;

    var slug = title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");

    if (editingId) {
      await db.update("posts", editingId, { title: title, body: body, slug: slug });
    } else {
      var myLabel = "";
      try { myLabel = await Goop.identity.label(); } catch (_) {}
      await db.insert("posts", { title: title, body: body, slug: slug, author_name: myLabel });
    }

    overlay.classList.add("hidden");
    loadPosts();
  });

  // ── Init ──
  await seed();
  loadPosts();
})();
