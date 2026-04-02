// Blog app.js
(async function () {
  var h = Goop.dom;
  var site = Goop.site;
  var date = Goop.date;
  var blog = Goop.data.api("blog");
  var editor = Goop.overlay("editor-overlay");

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
  var designerSidebar = null;

  function setupDesigner(cfg) {
    if (!canAdmin) return;
    var defaults = { layout: "list", blog_title: "My Blog", blog_subtitle: "Thoughts, stories & notes", accent: "#b44d2d", font: "serif", theme: "light" };
    for (var k in defaults) if (!cfg[k]) { cfg[k] = defaults[k]; blog("save_config", { key: k, value: defaults[k] }); }

    btnCustomize.classList.remove("hidden");

    designerSidebar = Goop.ui.sidebar({
      title: "Design",
      side: "right",
      width: 280,
      content: '<div class="designer-body">' +
        '<label class="designer-field">Blog Title<input type="text" id="d-title" placeholder="My Blog"></label>' +
        '<label class="designer-field">Subtitle<input type="text" id="d-subtitle" placeholder="Thoughts, stories &amp; notes"></label>' +
        '<div class="designer-section">Layout</div><div id="d-layout"></div>' +
        '<div class="designer-section">Font</div><div id="d-font"></div>' +
        '<div class="designer-section">Theme</div><div id="d-theme"></div>' +
        '<div class="designer-section">Accent Color</div><div id="d-accent"></div>' +
        '</div>',
    });

    var body = designerSidebar.body;
    body.querySelector("#d-title").value = cfg.blog_title || "My Blog";
    body.querySelector("#d-subtitle").value = cfg.blog_subtitle || "";

    Goop.ui.toolbar(body.querySelector("#d-layout"), {
      active: cfg.layout || "list",
      buttons: [
        { id: "list", label: "\u2630 List" },
        { id: "grid", label: "\u229E Grid" },
        { id: "magazine", label: "\u25A4 Magazine" },
      ],
      onChange: function(v) { cfg.layout = v; applyConfig(cfg); blog("save_config", { key: "layout", value: v }); },
    });

    Goop.ui.toolbar(body.querySelector("#d-font"), {
      active: cfg.font || "serif",
      buttons: [
        { id: "serif", label: "Aa Serif" },
        { id: "sans", label: "Aa Sans" },
        { id: "mono", label: "Aa Mono" },
      ],
      onChange: function(v) { cfg.font = v; applyConfig(cfg); blog("save_config", { key: "font", value: v }); },
    });

    Goop.ui.toolbar(body.querySelector("#d-theme"), {
      active: cfg.theme || "light",
      buttons: [
        { id: "light", label: "Light" },
        { id: "sepia", label: "Sepia" },
        { id: "dark", label: "Dark" },
      ],
      onChange: function(v) { cfg.theme = v; applyConfig(cfg); blog("save_config", { key: "theme", value: v }); },
    });

    Goop.ui.colorpicker(body.querySelector("#d-accent"), {
      value: cfg.accent || "#b44d2d",
      colors: ["#b44d2d", "#2d6a9f", "#4a8f46", "#7c4a9f", "#c0882c", "#2d7a6a"],
      showHex: false,
      onChange: function(v) { cfg.accent = v; applyConfig(cfg); blog("save_config", { key: "accent", value: v }); },
    });

    var titleInput = body.querySelector("#d-title");
    titleInput.addEventListener("blur", function () {
      cfg.blog_title = titleInput.value.trim() || "My Blog"; titleInput.value = cfg.blog_title;
      applyConfig(cfg); blog("save_config", { key: "blog_title", value: cfg.blog_title });
    });
    var subtitleInput = body.querySelector("#d-subtitle");
    subtitleInput.addEventListener("blur", function () {
      cfg.blog_subtitle = subtitleInput.value.trim();
      applyConfig(cfg); blog("save_config", { key: "blog_subtitle", value: cfg.blog_subtitle });
    });

    btnCustomize.addEventListener("click", function () { designerSidebar.open(); });
  }

  // ── Posts ──
  function renderPosts(rows) {
    Goop.list(postsEl, rows, function (p) {
      var slug = p.slug || p._id;
      var url = "post.html?slug=" + encodeURIComponent(slug);
      return h("article", { class: "post", data: { id: p._id } },
        p.image ? h("a", { href: url }, h("img", { class: "post-image", src: "images/" + p.image, alt: "" })) : null,
        h("h2", { class: "post-title" }, h("a", { class: "post-title-link", href: url }, p.title)),
        h("div", { class: "post-meta" }, date(p._created_at)),
        p.author_name ? h("div", { class: "post-byline" }, "by " + p.author_name) : null,
        h("div", { class: "post-body" }, p.body),
        h("a", { class: "post-read-more", href: url }, "Read more"),
        canWrite ? h("div", { class: "post-actions" },
          h("button", { onclick: function() { openEditor(p._id); } }, "Edit"),
          h("button", { onclick: async function() {
            if (!(await Goop.ui.confirm("Delete this post?"))) return;
            var result = await blog("delete_post", { id: p._id });
            if (result.image && canAdmin && site) { try { await site.remove("images/" + result.image); } catch (_) {} }
            reload();
          } }, "Delete")
        ) : null
      );
    }, {
      empty: "No posts yet." + (canWrite ? ' Click "+ New Post" to write your first one.' : "")
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
