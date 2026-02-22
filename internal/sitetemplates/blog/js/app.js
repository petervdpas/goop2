// Blog app.js
(async function () {
  var db = Goop.data;
  var site = Goop.site;
  var postsEl = document.getElementById("posts");
  var btnNew = document.getElementById("btn-new");
  var btnCustomize = document.getElementById("btn-customize");
  var designerPanel = document.getElementById("designer-panel");
  var overlay = document.getElementById("editor-overlay");

  var isOwner = false;
  var isCoAuthor = false;
  var myId = null;
  var currentLayout = "list";
  var configMap = {}; // key -> { id: number, value: string }
  var editingId = null;
  var editingImage = null; // filename of current post's image when editing

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  // ── Owner / co-author detection ──
  try {
    myId = await Goop.identity.id();
    var match = window.location.pathname.match(/\/p\/([^/]+)/);
    var ownerPeerId = match ? match[1] : null;
    if (ownerPeerId && ownerPeerId === myId) {
      isOwner = true;
    } else if (ownerPeerId && Goop.group) {
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
  if (isOwner) {
    document.getElementById("editor-image-section").classList.remove("hidden");
  }

  // ── Config helpers ──

  // Map hex → accent class index (avoids inline style / CSP issues)
  var accentToIdx = {
    "#b44d2d": "1", "#2d6a9f": "2", "#4a8f46": "3",
    "#7c4a9f": "4", "#c0882c": "5", "#2d7a6a": "6",
  };

  function applyConfigKey(key, value) {
    var html = document.documentElement;
    switch (key) {
      case "layout":
        currentLayout = value || "list";
        document.querySelector(".blog").className = "blog layout-" + currentLayout;
        document.querySelectorAll(".layout-btn").forEach(function (btn) {
          btn.classList.toggle("active", btn.dataset.layout === currentLayout);
        });
        break;
      case "blog_title":
        document.querySelector(".blog-title").textContent = value || "My Blog";
        break;
      case "blog_subtitle":
        document.getElementById("blog-subtitle").textContent =
          value || "Thoughts, stories & notes";
        break;
      case "accent":
        if (value) {
          // Toggle accent-N class — no inline style needed, fully CSS-driven
          html.className = html.className.replace(/\baccent-\d+\b/g, "").trim();
          var idx = accentToIdx[value] || "1";
          html.classList.add("accent-" + idx);
          document.querySelectorAll(".swatch").forEach(function (sw) {
            sw.classList.toggle("active", sw.dataset.color === value);
          });
        }
        break;
      case "font":
        html.className = html.className.replace(/\bfont-\w+\b/g, "").trim();
        html.classList.add("font-" + (value || "serif"));
        document.querySelectorAll(".font-btn").forEach(function (btn) {
          btn.classList.toggle("active", btn.dataset.font === (value || "serif"));
        });
        break;
      case "theme":
        html.className = html.className.replace(/\btheme-\w+\b/g, "").trim();
        html.classList.add("theme-" + (value || "light"));
        document.querySelectorAll(".theme-btn").forEach(function (btn) {
          btn.classList.toggle("active", btn.dataset.theme === (value || "light"));
        });
        break;
    }
  }

  async function saveConfig(key, value) {
    if (configMap[key] && configMap[key].id) {
      await db.update("blog_config", configMap[key].id, { value: value });
      configMap[key].value = value;
    } else {
      await db.insert("blog_config", { key: key, value: value });
      try {
        var rows = await db.query("blog_config", { where: "key = ?", args: [key], limit: 1 });
        if (rows && rows.length > 0) {
          configMap[key] = { id: rows[0]._id, value: value };
        }
      } catch (_) {}
    }
  }

  // ── Design panel setup (host only) ──
  async function setupDesigner() {
    // Load all config rows
    try {
      var rows = await db.query("blog_config", { limit: 100 });
      (rows || []).forEach(function (r) {
        configMap[r.key] = { id: r._id, value: r.value };
      });
    } catch (_) {}

    // Apply config to DOM (owner + visitors + co-authors all benefit)
    var loaded = {
      layout:        (configMap.layout        || {}).value,
      blog_title:    (configMap.blog_title    || {}).value,
      blog_subtitle: (configMap.blog_subtitle || {}).value,
      accent:        (configMap.accent        || {}).value,
      font:          (configMap.font          || {}).value,
      theme:         (configMap.theme         || {}).value,
    };
    applyConfigKey("layout",        loaded.layout);
    applyConfigKey("blog_title",    loaded.blog_title);
    applyConfigKey("blog_subtitle", loaded.blog_subtitle);
    applyConfigKey("accent",        loaded.accent);
    applyConfigKey("font",          loaded.font);
    applyConfigKey("theme",         loaded.theme);

    if (!isOwner) return; // visitors and co-authors stop here

    // Ensure default config rows exist
    var defaults = {
      layout:        "list",
      blog_title:    "My Blog",
      blog_subtitle: "Thoughts, stories & notes",
      accent:        "#b44d2d",
      font:          "serif",
      theme:         "light",
    };
    for (var k in defaults) {
      if (!configMap[k]) {
        await saveConfig(k, defaults[k]);
        applyConfigKey(k, defaults[k]);
      }
    }

    // Populate designer inputs
    document.getElementById("d-title").value =
      (configMap.blog_title || {}).value || "My Blog";
    document.getElementById("d-subtitle").value =
      (configMap.blog_subtitle || {}).value || "";

    // Show customize button
    btnCustomize.classList.remove("hidden");

    // ── Wire layout buttons ──
    document.querySelectorAll(".layout-btn").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        applyConfigKey("layout", btn.dataset.layout);
        await saveConfig("layout", btn.dataset.layout);
      });
    });

    // ── Wire color swatches ──
    document.querySelectorAll(".swatch").forEach(function (sw) {
      sw.addEventListener("click", async function () {
        applyConfigKey("accent", sw.dataset.color);
        await saveConfig("accent", sw.dataset.color);
      });
    });

    // ── Wire font buttons ──
    document.querySelectorAll(".font-btn").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        applyConfigKey("font", btn.dataset.font);
        await saveConfig("font", btn.dataset.font);
      });
    });

    // ── Wire theme buttons ──
    document.querySelectorAll(".theme-btn").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        applyConfigKey("theme", btn.dataset.theme);
        await saveConfig("theme", btn.dataset.theme);
      });
    });

    // ── Wire title input ──
    var titleInput = document.getElementById("d-title");
    titleInput.addEventListener("input", function () {
      applyConfigKey("blog_title", titleInput.value || "My Blog");
    });
    titleInput.addEventListener("blur", async function () {
      var val = titleInput.value.trim() || "My Blog";
      titleInput.value = val;
      await saveConfig("blog_title", val);
    });

    // ── Wire subtitle input ──
    var subtitleInput = document.getElementById("d-subtitle");
    subtitleInput.addEventListener("input", function () {
      applyConfigKey("blog_subtitle", subtitleInput.value);
    });
    subtitleInput.addEventListener("blur", async function () {
      await saveConfig("blog_subtitle", subtitleInput.value.trim());
    });

    // ── Wire panel open / close ──
    btnCustomize.addEventListener("click", function () {
      designerPanel.classList.toggle("hidden");
    });
    document.getElementById("btn-designer-close").addEventListener("click", function () {
      designerPanel.classList.add("hidden");
    });
  }

  // ── Seed sample posts on first run ──
  async function seed() {
    var tables = await db.tables();
    if (tables && tables.length > 0) return;

    await db.createTable("posts", [
      { name: "title",       type: "TEXT",    not_null: true },
      { name: "body",        type: "TEXT",    not_null: true },
      { name: "author_name", type: "TEXT",    default: "" },
      { name: "slug",        type: "TEXT" },
      { name: "published",   type: "INTEGER", default: "1" },
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

  // ── Load & render posts ──
  async function loadPosts() {
    try {
      var rows = await db.query("posts", { where: "published = 1", limit: 50 });
      renderPosts(rows || []);
    } catch (err) {
      postsEl.innerHTML =
        '<div class="empty-msg"><p>Could not load posts.</p><p class="loading">' +
        esc(err.message) + "</p></div>";
    }
  }

  function renderPosts(posts) {
    if (posts.length === 0) {
      postsEl.innerHTML =
        '<div class="empty-msg"><p>No posts yet.</p>' +
        ((isOwner || isCoAuthor)
          ? '<p class="loading">Click "+ New Post" to write your first one.</p>'
          : "") +
        "</div>";
      return;
    }

    posts.sort(function (a, b) { return b._id - a._id; });

    postsEl.innerHTML = posts.map(function (p) {
      var date = p._created_at
        ? new Date(String(p._created_at).replace(" ", "T") + "Z")
            .toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })
        : "";
      var html = '<article class="post">';
      if (p.image) {
        html += '<img class="post-image" src="images/' + esc(p.image) + '" alt="">';
      }
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
        html += '<button data-action="delete" data-id="' + p._id + '" data-image="' + esc(p.image || "") + '">Delete</button>';
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
        openEditor(parseInt(btn.getAttribute("data-id"), 10));
      });
    });
    postsEl.querySelectorAll("[data-action=delete]").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        var id = parseInt(btn.getAttribute("data-id"), 10);
        var imgFile = btn.getAttribute("data-image") || "";
        var ok = true;
        if (Goop.ui) ok = await Goop.ui.confirm("Delete this post?");
        if (!ok) return;
        await db.remove("posts", id);
        if (imgFile && isOwner && site) {
          try { await site.remove("images/" + imgFile); } catch (_) {}
        }
        loadPosts();
      });
    });
  }

  // ── Editor ──
  async function openEditor(id) {
    editingId = id || null;
    editingImage = null;
    document.getElementById("f-title").value = "";
    document.getElementById("f-body").value = "";
    document.getElementById("editor-heading").textContent = id ? "Edit Post" : "New Post";
    document.getElementById("btn-save").textContent = id ? "Update" : "Publish";

    // Reset image input + preview
    var fImage = document.getElementById("f-image");
    var fPreview = document.getElementById("f-image-preview");
    if (fImage) fImage.value = "";
    if (fPreview) { fPreview.src = ""; fPreview.classList.add("hidden"); }

    if (id) {
      try {
        var rows = await db.query("posts", { where: "_id = ?", args: [id], limit: 1 });
        if (rows && rows.length > 0) {
          document.getElementById("f-title").value = rows[0].title;
          document.getElementById("f-body").value = rows[0].body;
          if (rows[0].image && fPreview) {
            editingImage = rows[0].image;
            fPreview.src = "images/" + editingImage;
            fPreview.classList.remove("hidden");
          }
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

    // Handle image upload (owner only)
    var imageName = editingId ? editingImage : "";
    var fImage = document.getElementById("f-image");
    var imageFile = fImage && fImage.files && fImage.files[0];
    if (imageFile && isOwner && site) {
      var ext = (imageFile.name.split(".").pop() || "jpg").toLowerCase().replace(/[^a-z0-9]/g, "");
      var safeName = Date.now() + "-" +
        imageFile.name.replace(/\.[^.]+$/, "").replace(/[^a-zA-Z0-9_-]/g, "_").substring(0, 60) +
        "." + ext;
      try {
        await site.upload("images/" + safeName, imageFile);
        // Remove old image if replacing
        if (editingImage) {
          try { await site.remove("images/" + editingImage); } catch (_) {}
        }
        imageName = safeName;
      } catch (_) {
        // Upload failed — save post without image change
      }
    }

    if (editingId) {
      await db.update("posts", editingId, { title: title, body: body, slug: slug, image: imageName || "" });
    } else {
      var myLabel = "";
      try { myLabel = await Goop.identity.label(); } catch (_) {}
      await db.insert("posts", { title: title, body: body, slug: slug, author_name: myLabel, image: imageName || "" });
    }

    overlay.classList.add("hidden");
    loadPosts();
  });

  // ── Init ──
  await setupDesigner(); // loads + applies config for everyone; wires controls for owner
  await seed();
  loadPosts();
})();
