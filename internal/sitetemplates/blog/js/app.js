// Blog app.js
(async function () {
  var h = Goop.dom;
  var site = Goop.site;
  var date = Goop.date;
  var blog = Goop.data.api("blog");
  var editor = Goop.overlay("editor-overlay");

  Goop.ui.dialog(document.getElementById("confirm-dialog"), {
    title: ".gc-dialog-title",
    message: ".gc-dialog-message",
    inputWrap: ".gc-dialog-input-wrap",
    input: ".gc-dialog-input",
    ok: ".gc-dialog-ok",
    cancel: ".gc-dialog-cancel",
    hiddenClass: "hidden",
  });

  var postsEl = document.getElementById("posts");
  var btnNew = document.getElementById("btn-new");
  var btnCustomize = document.getElementById("btn-customize");
  var designerPanel = document.getElementById("designer-panel");

  var editingId = null;
  var editingImage = null;
  var canWrite = false;
  var canAdmin = false;

  // ── Config ──

  var accentToIdx = {
    "#b44d2d": "1", "#2d6a9f": "2", "#4a8f46": "3",
    "#7c4a9f": "4", "#c0882c": "5", "#2d7a6a": "6",
  };

  function applyConfig(cfg) {
    var html = document.documentElement;
    document.querySelector(".blog").className = "blog layout-" + (cfg.layout || "list");
    document.querySelectorAll(".layout-btn").forEach(function (b) { b.classList.toggle("active", b.dataset.layout === (cfg.layout || "list")); });
    document.querySelector(".blog-title").textContent = cfg.blog_title || "My Blog";
    document.getElementById("blog-subtitle").textContent = cfg.blog_subtitle || "Thoughts, stories & notes";
    if (cfg.accent) {
      html.className = html.className.replace(/\baccent-\d+\b/g, "").trim();
      html.classList.add("accent-" + (accentToIdx[cfg.accent] || "1"));
      document.querySelectorAll(".swatch").forEach(function (s) { s.classList.toggle("active", s.dataset.color === cfg.accent); });
    }
    html.className = html.className.replace(/\bfont-\w+\b/g, "").trim();
    html.classList.add("font-" + (cfg.font || "serif"));
    document.querySelectorAll(".font-btn").forEach(function (b) { b.classList.toggle("active", b.dataset.font === (cfg.font || "serif")); });
    html.className = html.className.replace(/\btheme-\w+\b/g, "").trim();
    html.classList.add("theme-" + (cfg.theme || "light"));
    document.querySelectorAll(".theme-btn").forEach(function (b) { b.classList.toggle("active", b.dataset.theme === (cfg.theme || "light")); });
  }

  // ── Designer (owner only) ──
  function setupDesigner(cfg) {
    if (!canAdmin) return;
    var defaults = { layout: "list", blog_title: "My Blog", blog_subtitle: "Thoughts, stories & notes", accent: "#b44d2d", font: "serif", theme: "light" };
    for (var k in defaults) if (!cfg[k]) { cfg[k] = defaults[k]; blog("save_config", { key: k, value: defaults[k] }); }

    var panel = document.getElementById("designer-panel");
    document.getElementById("d-title").value = cfg.blog_title || "My Blog";
    document.getElementById("d-subtitle").value = cfg.blog_subtitle || "";
    btnCustomize.classList.remove("hidden");

    function bind(sel, attr, key) {
      document.querySelectorAll(sel).forEach(function (b) {
        b.addEventListener("click", function () { cfg[key] = b.dataset[attr]; applyConfig(cfg); blog("save_config", { key: key, value: cfg[key] }); });
      });
    }
    bind(".layout-btn[data-layout]", "layout", "layout");
    bind(".swatch", "color", "accent");
    bind(".font-btn", "font", "font");
    bind(".theme-btn", "theme", "theme");

    var titleInput = document.getElementById("d-title");
    titleInput.addEventListener("blur", function () {
      cfg.blog_title = titleInput.value.trim() || "My Blog"; titleInput.value = cfg.blog_title;
      applyConfig(cfg); blog("save_config", { key: "blog_title", value: cfg.blog_title });
    });
    var subtitleInput = document.getElementById("d-subtitle");
    subtitleInput.addEventListener("blur", function () {
      cfg.blog_subtitle = subtitleInput.value.trim();
      applyConfig(cfg); blog("save_config", { key: "blog_subtitle", value: cfg.blog_subtitle });
    });

    btnCustomize.addEventListener("click", function () { panel.classList.toggle("hidden"); });
    document.getElementById("btn-designer-close").addEventListener("click", function () { panel.classList.add("hidden"); });
  }

  // ── Posts ──
  function renderPosts(rows) {
    var postsData = rows.map(function(p) {
      return { _id: p._id, slug: p.slug || p._id, title: p.title, body: p.body, image: p.image, author_name: p.author_name, date: date(p._created_at) };
    });
    Goop.list(postsEl, postsData, "post-card", {
      empty: "No posts yet." + (canWrite ? ' Click "+ New Post" to write your first one.' : "")
    }).then(function() {
      if (!canWrite) return;
      postsEl.querySelectorAll("[data-id]").forEach(function(article) {
        var id = parseInt(article.dataset.id, 10);
        var p = rows.find(function(r) { return r._id === id; });
        var actions = h("div", { class: "post-actions" },
          h("button", { onclick: function() { openEditor(id); } }, "Edit"),
          h("button", { onclick: async function() {
            if (!(await Goop.ui.confirm("Delete this post?"))) return;
            var result = await blog("delete_post", { id: id });
            if (result.image && canAdmin && site) { try { await site.remove("images/" + result.image); } catch (_) {} }
            reload();
          } }, "Delete")
        );
        article.appendChild(actions);
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
        var r = await blog("get_post", { slug: String(id) });
        if (r.found && r.post) {
          document.getElementById("f-title").value = r.post.title;
          document.getElementById("f-body").value = r.post.body;
          if (r.post.image && fPreview) { editingImage = r.post.image; fPreview.src = "images/" + editingImage; fPreview.classList.remove("hidden"); }
        }
      } catch (_) {}
    }
    editor.open();
  }

  btnNew.addEventListener("click", function () { openEditor(null); });
  document.getElementById("btn-cancel").addEventListener("click", function () { editor.close(); });

  document.getElementById("btn-save").addEventListener("click", async function () {
    var title = document.getElementById("f-title").value.trim();
    var body = document.getElementById("f-body").value.trim();
    if (!title || !body) return;

    var imageName = editingId ? (editingImage || "") : "";
    var fImage = document.getElementById("f-image");
    var imageFile = fImage && fImage.files && fImage.files[0];
    if (imageFile && canAdmin && site) {
      var ext = (imageFile.name.split(".").pop() || "jpg").toLowerCase().replace(/[^a-z0-9]/g, "");
      var safeName = Date.now() + "-" + imageFile.name.replace(/\.[^.]+$/, "").replace(/[^a-zA-Z0-9_-]/g, "_").substring(0, 60) + "." + ext;
      try {
        await site.upload("images/" + safeName, imageFile);
        if (editingImage) { try { await site.remove("images/" + editingImage); } catch (_) {} }
        imageName = safeName;
      } catch (_) {}
    }

    await blog("save_post", { id: editingId, title: title, body: body, image: imageName });
    editor.close();
    reload();
  });

  // ── Init ──
  async function reload() {
    var p = await blog("page");
    renderPosts(p.posts || []);
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
