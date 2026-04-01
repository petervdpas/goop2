// Blog app.js — all data via Lua, JS is pure rendering
(async function () {
  var db = Goop.data;
  var site = Goop.site;
  var esc = Goop.esc;
  var date = Goop.date;

  function blog(action, params) {
    return db.call("blog", Object.assign({ action: action }, params || {}));
  }

  var postsEl = document.getElementById("posts");
  var btnNew = document.getElementById("btn-new");
  var btnCustomize = document.getElementById("btn-customize");
  var designerPanel = document.getElementById("designer-panel");
  var overlay = document.getElementById("editor-overlay");

  var currentLayout = "list";
  var editingId = null;
  var editingImage = null;
  var canWrite = false;
  var canAdmin = false;

  // ── Config helpers ──

  var accentToIdx = {
    "#b44d2d": "1", "#2d6a9f": "2", "#4a8f46": "3",
    "#7c4a9f": "4", "#c0882c": "5", "#2d7a6a": "6",
  };

  function applyConfig(cfg) {
    var html = document.documentElement;

    currentLayout = cfg.layout || "list";
    document.querySelector(".blog").className = "blog layout-" + currentLayout;
    document.querySelectorAll(".layout-btn").forEach(function (btn) {
      btn.classList.toggle("active", btn.dataset.layout === currentLayout);
    });

    document.querySelector(".blog-title").textContent = cfg.blog_title || "My Blog";
    document.getElementById("blog-subtitle").textContent = cfg.blog_subtitle || "Thoughts, stories & notes";

    if (cfg.accent) {
      html.className = html.className.replace(/\baccent-\d+\b/g, "").trim();
      html.classList.add("accent-" + (accentToIdx[cfg.accent] || "1"));
      document.querySelectorAll(".swatch").forEach(function (sw) {
        sw.classList.toggle("active", sw.dataset.color === cfg.accent);
      });
    }

    html.className = html.className.replace(/\bfont-\w+\b/g, "").trim();
    html.classList.add("font-" + (cfg.font || "serif"));
    document.querySelectorAll(".font-btn").forEach(function (btn) {
      btn.classList.toggle("active", btn.dataset.font === (cfg.font || "serif"));
    });

    html.className = html.className.replace(/\btheme-\w+\b/g, "").trim();
    html.classList.add("theme-" + (cfg.theme || "light"));
    document.querySelectorAll(".theme-btn").forEach(function (btn) {
      btn.classList.toggle("active", btn.dataset.theme === (cfg.theme || "light"));
    });
  }

  function saveCfg(key, value) {
    return blog("save_config", { key: key, value: value });
  }

  // ── Design panel (host only) ──
  function setupDesigner(cfg) {
    if (!canAdmin) return;

    var defaults = {
      layout: "list", blog_title: "My Blog", blog_subtitle: "Thoughts, stories & notes",
      accent: "#b44d2d", font: "serif", theme: "light",
    };
    var saves = [];
    for (var k in defaults) {
      if (!cfg[k]) { cfg[k] = defaults[k]; saves.push(saveCfg(k, defaults[k])); }
    }

    document.getElementById("d-title").value = cfg.blog_title || "My Blog";
    document.getElementById("d-subtitle").value = cfg.blog_subtitle || "";

    btnCustomize.classList.remove("hidden");

    document.querySelectorAll(".layout-btn").forEach(function (btn) {
      btn.addEventListener("click", function () {
        cfg.layout = btn.dataset.layout; applyConfig(cfg); saveCfg("layout", cfg.layout);
      });
    });

    document.querySelectorAll(".swatch").forEach(function (sw) {
      sw.addEventListener("click", function () {
        cfg.accent = sw.dataset.color; applyConfig(cfg); saveCfg("accent", cfg.accent);
      });
    });

    document.querySelectorAll(".font-btn").forEach(function (btn) {
      btn.addEventListener("click", function () {
        cfg.font = btn.dataset.font; applyConfig(cfg); saveCfg("font", cfg.font);
      });
    });

    document.querySelectorAll(".theme-btn").forEach(function (btn) {
      btn.addEventListener("click", function () {
        cfg.theme = btn.dataset.theme; applyConfig(cfg); saveCfg("theme", cfg.theme);
      });
    });

    var titleInput = document.getElementById("d-title");
    titleInput.addEventListener("blur", function () {
      var val = titleInput.value.trim() || "My Blog";
      titleInput.value = val;
      cfg.blog_title = val; applyConfig(cfg); saveCfg("blog_title", val);
    });

    var subtitleInput = document.getElementById("d-subtitle");
    subtitleInput.addEventListener("blur", function () {
      cfg.blog_subtitle = subtitleInput.value.trim();
      applyConfig(cfg); saveCfg("blog_subtitle", cfg.blog_subtitle);
    });

    btnCustomize.addEventListener("click", function () { designerPanel.classList.toggle("hidden"); });
    document.getElementById("btn-designer-close").addEventListener("click", function () { designerPanel.classList.add("hidden"); });
  }

  // ── Render posts ──
  function renderPosts(rows) {
    if (rows.length === 0) {
      postsEl.innerHTML =
        '<div class="empty-msg"><p>No posts yet.</p>' +
        (canWrite ? '<p class="loading">Click "+ New Post" to write your first one.</p>' : "") +
        "</div>";
      return;
    }

    postsEl.innerHTML = rows.map(function (p) {
      var slug = p.slug || p._id;
      var postUrl = 'post.html?slug=' + encodeURIComponent(slug);
      var html = '<article class="post">';
      if (p.image) html += '<a href="' + postUrl + '"><img class="post-image" src="images/' + esc(p.image) + '" alt=""></a>';
      html += '<h2 class="post-title"><a class="post-title-link" href="' + postUrl + '">' + esc(p.title) + "</a></h2>";
      html += '<div class="post-meta">' + date(p._created_at) + "</div>";
      if (p.author_name) html += '<div class="post-byline">by ' + esc(p.author_name) + "</div>";
      html += '<div class="post-body">' + esc(p.body) + "</div>";
      html += '<a class="post-read-more" href="' + postUrl + '">Read more</a>';
      if (canWrite) {
        html += '<div class="post-actions">';
        html += '<button data-action="edit" data-id="' + p._id + '">Edit</button>';
        html += '<button data-action="delete" data-id="' + p._id + '" data-image="' + esc(p.image || "") + '">Delete</button>';
        html += "</div>";
      }
      html += "</article>";
      return html;
    }).join("");

    if (canWrite) wireActions();
  }

  function wireActions() {
    postsEl.querySelectorAll("[data-action=edit]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        openEditor(parseInt(btn.dataset.id, 10));
      });
    });
    postsEl.querySelectorAll("[data-action=delete]").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        var id = parseInt(btn.dataset.id, 10);
        var ok = true;
        if (Goop.ui) ok = await Goop.ui.confirm("Delete this post?");
        if (!ok) return;
        var result = await blog("delete_post", { id: id });
        if (result.image && canAdmin && site) {
          try { await site.remove("images/" + result.image); } catch (_) {}
        }
        reload();
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

    var fImage = document.getElementById("f-image");
    var fPreview = document.getElementById("f-image-preview");
    if (fImage) fImage.value = "";
    if (fPreview) { fPreview.src = ""; fPreview.classList.add("hidden"); }

    if (id) {
      try {
        var result = await blog("get_post", { slug: String(id) });
        if (result.found && result.post) {
          document.getElementById("f-title").value = result.post.title;
          document.getElementById("f-body").value = result.post.body;
          if (result.post.image && fPreview) {
            editingImage = result.post.image;
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

    var imageName = editingId ? (editingImage || "") : "";
    var fImage = document.getElementById("f-image");
    var imageFile = fImage && fImage.files && fImage.files[0];
    if (imageFile && canAdmin && site) {
      var ext = (imageFile.name.split(".").pop() || "jpg").toLowerCase().replace(/[^a-z0-9]/g, "");
      var safeName = Date.now() + "-" +
        imageFile.name.replace(/\.[^.]+$/, "").replace(/[^a-zA-Z0-9_-]/g, "_").substring(0, 60) +
        "." + ext;
      try {
        await site.upload("images/" + safeName, imageFile);
        if (editingImage) { try { await site.remove("images/" + editingImage); } catch (_) {} }
        imageName = safeName;
      } catch (_) {}
    }

    await blog("save_post", { id: editingId, title: title, body: body, image: imageName });
    overlay.classList.add("hidden");
    reload();
  });

  // ── Init ──
  async function reload() {
    var page = await blog("page");
    renderPosts(page.posts || []);
  }

  var page = await blog("page");
  canWrite = page.can_write;
  canAdmin = page.can_admin;

  if (canWrite) btnNew.classList.remove("hidden");
  if (canAdmin) document.getElementById("editor-image-section").classList.remove("hidden");

  applyConfig(page.config || {});
  setupDesigner(page.config || {});
  renderPosts(page.posts || []);
})();
