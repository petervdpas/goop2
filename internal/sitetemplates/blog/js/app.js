// Blog app.js — ORM DSL
(async function () {
  var db = Goop.data;
  var site = Goop.site;
  var esc = Goop.esc;
  var ctx = await Goop.peer();

  var posts = await db.orm("posts");
  var config = await db.orm("blog_config");

  var postsEl = document.getElementById("posts");
  var btnNew = document.getElementById("btn-new");
  var btnCustomize = document.getElementById("btn-customize");
  var designerPanel = document.getElementById("designer-panel");
  var overlay = document.getElementById("editor-overlay");

  var currentLayout = "list";
  var configMap = {};
  var editingId = null;
  var editingImage = null;

  var canWrite = ctx.isOwner || ctx.isGroup;
  if (canWrite) btnNew.classList.remove("hidden");
  if (ctx.isOwner) document.getElementById("editor-image-section").classList.remove("hidden");

  // ── Config helpers ──

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
    var result = await config.upsert("key", { key: key, value: value });
    configMap[key] = { id: result.id, value: value };
  }

  // ── Design panel setup (host only) ──
  async function setupDesigner() {
    try {
      var rows = await config.find();
      (rows || []).forEach(function (r) {
        configMap[r.key] = { id: r._id, value: r.value };
      });
    } catch (_) {}

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

    if (!ctx.isOwner) return;

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

    document.getElementById("d-title").value =
      (configMap.blog_title || {}).value || "My Blog";
    document.getElementById("d-subtitle").value =
      (configMap.blog_subtitle || {}).value || "";

    btnCustomize.classList.remove("hidden");

    document.querySelectorAll(".layout-btn").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        applyConfigKey("layout", btn.dataset.layout);
        await saveConfig("layout", btn.dataset.layout);
      });
    });

    document.querySelectorAll(".swatch").forEach(function (sw) {
      sw.addEventListener("click", async function () {
        applyConfigKey("accent", sw.dataset.color);
        await saveConfig("accent", sw.dataset.color);
      });
    });

    document.querySelectorAll(".font-btn").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        applyConfigKey("font", btn.dataset.font);
        await saveConfig("font", btn.dataset.font);
      });
    });

    document.querySelectorAll(".theme-btn").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        applyConfigKey("theme", btn.dataset.theme);
        await saveConfig("theme", btn.dataset.theme);
      });
    });

    var titleInput = document.getElementById("d-title");
    titleInput.addEventListener("input", function () {
      applyConfigKey("blog_title", titleInput.value || "My Blog");
    });
    titleInput.addEventListener("blur", async function () {
      var val = titleInput.value.trim() || "My Blog";
      titleInput.value = val;
      await saveConfig("blog_title", val);
    });

    var subtitleInput = document.getElementById("d-subtitle");
    subtitleInput.addEventListener("input", function () {
      applyConfigKey("blog_subtitle", subtitleInput.value);
    });
    subtitleInput.addEventListener("blur", async function () {
      await saveConfig("blog_subtitle", subtitleInput.value.trim());
    });

    btnCustomize.addEventListener("click", function () {
      designerPanel.classList.toggle("hidden");
    });
    document.getElementById("btn-designer-close").addEventListener("click", function () {
      designerPanel.classList.add("hidden");
    });
  }

  // ── Load & render posts ──
  async function loadPosts() {
    try {
      var rows = await posts.find({ where: "published = 1", order: "_id DESC", limit: 50 });
      renderPosts(rows || []);
    } catch (err) {
      postsEl.innerHTML =
        '<div class="empty-msg"><p>Could not load posts.</p><p class="loading">' +
        esc(err.message) + "</p></div>";
    }
  }

  function renderPosts(rows) {
    if (rows.length === 0) {
      postsEl.innerHTML =
        '<div class="empty-msg"><p>No posts yet.</p>' +
        (canWrite
          ? '<p class="loading">Click "+ New Post" to write your first one.</p>'
          : "") +
        "</div>";
      return;
    }

    postsEl.innerHTML = rows.map(function (p) {
      var date = p._created_at
        ? new Date(String(p._created_at).replace(" ", "T") + "Z")
            .toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })
        : "";
      var slug = p.slug || p._id;
      var postUrl = 'post.html?slug=' + encodeURIComponent(slug);
      var html = '<article class="post">';
      if (p.image) {
        html += '<a href="' + postUrl + '"><img class="post-image" src="images/' + esc(p.image) + '" alt=""></a>';
      }
      html += '<h2 class="post-title"><a class="post-title-link" href="' + postUrl + '">' + esc(p.title) + "</a></h2>";
      html += '<div class="post-meta">' + esc(date) + "</div>";
      if (p.author_name) {
        html += '<div class="post-byline">by ' + esc(p.author_name) + "</div>";
      }
      html += '<div class="post-body">' + esc(p.body) + "</div>";
      html += '<a class="post-read-more" href="' + postUrl + '">Read more</a>';
      var canEdit = ctx.isOwner || (ctx.isGroup && p._owner === ctx.myId);
      if (canEdit) {
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
        await posts.remove(id);
        if (imgFile && ctx.isOwner && site) {
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

    var fImage = document.getElementById("f-image");
    var fPreview = document.getElementById("f-image-preview");
    if (fImage) fImage.value = "";
    if (fPreview) { fPreview.src = ""; fPreview.classList.add("hidden"); }

    if (id) {
      try {
        var row = await posts.findOne({ where: "_id = ?", args: [id] });
        if (row) {
          document.getElementById("f-title").value = row.title;
          document.getElementById("f-body").value = row.body;
          if (row.image && fPreview) {
            editingImage = row.image;
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

    var imageName = editingId ? editingImage : "";
    var fImage = document.getElementById("f-image");
    var imageFile = fImage && fImage.files && fImage.files[0];
    if (imageFile && ctx.isOwner && site) {
      var ext = (imageFile.name.split(".").pop() || "jpg").toLowerCase().replace(/[^a-z0-9]/g, "");
      var safeName = Date.now() + "-" +
        imageFile.name.replace(/\.[^.]+$/, "").replace(/[^a-zA-Z0-9_-]/g, "_").substring(0, 60) +
        "." + ext;
      try {
        await site.upload("images/" + safeName, imageFile);
        if (editingImage) {
          try { await site.remove("images/" + editingImage); } catch (_) {}
        }
        imageName = safeName;
      } catch (_) {}
    }

    if (editingId) {
      await posts.update(editingId, { title: title, body: body, slug: slug, image: imageName || "" });
    } else {
      await posts.insert({ title: title, body: body, slug: slug, author_name: ctx.label, image: imageName || "" });
    }

    overlay.classList.add("hidden");
    loadPosts();
  });

  // ── Init ──
  await setupDesigner();
  loadPosts();
})();
