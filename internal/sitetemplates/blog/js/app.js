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
    var root = document.documentElement;
    document.querySelector(".blog").className = "blog layout-" + (cfg.layout || "list");
    document.querySelector(".blog-title").textContent = cfg.blog_title || "My Blog";
    document.getElementById("blog-subtitle").textContent = cfg.blog_subtitle || "Thoughts, stories & notes";
    if (cfg.accent) {
      root.className = root.className.replace(/\baccent-\d+\b/g, "").trim();
      root.classList.add("accent-" + (accentToIdx[cfg.accent] || "1"));
    }
    root.className = root.className.replace(/\bfont-\w+\b/g, "").trim();
    root.classList.add("font-" + (cfg.font || "serif"));
    root.className = root.className.replace(/\btheme-\w+\b/g, "").trim();
    root.classList.add("theme-" + (cfg.theme || "light"));
  }

  // ── Designer (owner only) ──
  function setupDesigner(cfg) {
    if (!canAdmin) return;
    var defaults = { layout: "list", blog_title: "My Blog", blog_subtitle: "Thoughts, stories & notes", accent: "#b44d2d", font: "serif", theme: "light" };
    for (var k in defaults) if (!cfg[k]) { cfg[k] = defaults[k]; blog("save_config", { key: k, value: defaults[k] }); }

    document.getElementById("d-title").value = cfg.blog_title || "My Blog";
    document.getElementById("d-subtitle").value = cfg.blog_subtitle || "";
    btnCustomize.classList.remove("hidden");

    Goop.ui.toolbar(document.querySelector(".layout-picker"), {
      idAttr: "data-layout",
      activeClass: "active",
      active: cfg.layout || "list",
      onChange: function(val) { cfg.layout = val; applyConfig(cfg); blog("save_config", { key: "layout", value: val }); },
    });

    Goop.ui.toolbar(document.getElementById("font-picker"), {
      idAttr: "data-font",
      activeClass: "active",
      active: cfg.font || "serif",
      onChange: function(val) { cfg.font = val; applyConfig(cfg); blog("save_config", { key: "font", value: val }); },
    });

    Goop.ui.toolbar(document.getElementById("theme-picker"), {
      idAttr: "data-theme",
      activeClass: "active",
      active: cfg.theme || "light",
      onChange: function(val) { cfg.theme = val; applyConfig(cfg); blog("save_config", { key: "theme", value: val }); },
    });

    Goop.ui.toolbar(document.querySelector(".color-swatches"), {
      idAttr: "data-color",
      activeClass: "active",
      active: cfg.accent || "#b44d2d",
      onChange: function(val) { cfg.accent = val; applyConfig(cfg); blog("save_config", { key: "accent", value: val }); },
    });

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

    btnCustomize.addEventListener("click", function () { designerPanel.classList.toggle("hidden"); });
    document.getElementById("btn-designer-close").addEventListener("click", function () { designerPanel.classList.add("hidden"); });
  }

  // ── Posts ──
  function renderPosts(rows) {
    var postsData = rows.map(function(p) {
      return { _id: p._id, slug: p.slug || p._id, title: p.title, body: p.body, image: p.image, author_name: p.author_name, date: date(p._created_at) };
    });
    Goop.list(postsEl, postsData, "post-card", {
      empty: "No posts yet." + (canWrite ? ' Click "+ New Post" to write your first one.' : ""),
      emptyClass: "empty-msg"
    }).then(function() {
      if (!canWrite) return;
      postsEl.querySelectorAll("[data-id]").forEach(function(article) {
        var id = parseInt(article.dataset.id, 10);
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
