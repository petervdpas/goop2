// Blog post detail page
(async function () {
  var h = Goop.dom;
  var date = Goop.date;
  var blog = Goop.data.api("blog");
  var articleEl = document.getElementById("article");

  var basePath = "";
  var m = window.location.pathname.match(/^(\/p\/[^/]+)\//);
  if (m) basePath = m[1];

  function goHome() { window.location.href = basePath + "/"; }
  function backLink() {
    return h("a", { class: "article-back", href: "index.html", onclick: function(e) { e.preventDefault(); goHome(); } }, "Back");
  }

  try {
    var cfg = await blog("get_config");
    var root = document.documentElement;
    var accentToIdx = { "#b44d2d": "1", "#2d6a9f": "2", "#4a8f46": "3", "#7c4a9f": "4", "#c0882c": "5", "#2d7a6a": "6" };
    if (cfg.accent) { root.className = root.className.replace(/\baccent-\d+\b/g, "").trim(); root.classList.add("accent-" + (accentToIdx[cfg.accent] || "1")); }
    if (cfg.font) { root.className = root.className.replace(/\bfont-\w+\b/g, "").trim(); root.classList.add("font-" + cfg.font); }
    if (cfg.theme) { root.className = root.className.replace(/\btheme-\w+\b/g, "").trim(); root.classList.add("theme-" + cfg.theme); }
    if (cfg.blog_title) document.title = cfg.blog_title;
  } catch (_) {}

  var slug = new URLSearchParams(window.location.search).get("slug");
  if (!slug) { goHome(); return; }

  try {
    var result = await blog("get_post", { slug: slug });
    if (!result.found) {
      Goop.render(articleEl, backLink(), Goop.ui.empty("Post not found", { class: "empty-msg" }));
      return;
    }
    var p = result.post;
    document.title = p.title;
    var metaText = date(p._created_at) + (p.author_name ? " \u00B7 by " + p.author_name : "");
    Goop.render(articleEl,
      backLink(),
      p.image ? h("img", { class: "article-image", src: "images/" + p.image, alt: "" }) : null,
      h("h1", { class: "article-title" }, p.title),
      h("div", { class: "article-meta" }, metaText),
      h("div", { class: "article-body" }, p.body)
    );
  } catch (err) {
    Goop.render(articleEl, backLink(), Goop.ui.empty(err.message, { class: "empty-msg" }));
  }
})();
