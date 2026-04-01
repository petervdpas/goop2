// Blog post detail page
(async function () {
  var esc = Goop.esc;
  var date = Goop.date;
  var blog = Goop.data.api("blog");
  var articleEl = document.getElementById("article");

  var basePath = '';
  var m = window.location.pathname.match(/^(\/p\/[^/]+)\//);
  if (m) basePath = m[1];

  function goHome() { window.location.href = basePath + '/'; }
  function wireBack() {
    articleEl.querySelectorAll(".article-back").forEach(function (a) {
      a.addEventListener("click", function (e) { e.preventDefault(); goHome(); });
    });
  }

  try {
    var cfg = await blog("get_config");
    var html = document.documentElement;
    var accentToIdx = { "#b44d2d": "1", "#2d6a9f": "2", "#4a8f46": "3", "#7c4a9f": "4", "#c0882c": "5", "#2d7a6a": "6" };
    if (cfg.accent) { html.className = html.className.replace(/\baccent-\d+\b/g, "").trim(); html.classList.add("accent-" + (accentToIdx[cfg.accent] || "1")); }
    if (cfg.font) { html.className = html.className.replace(/\bfont-\w+\b/g, "").trim(); html.classList.add("font-" + cfg.font); }
    if (cfg.theme) { html.className = html.className.replace(/\btheme-\w+\b/g, "").trim(); html.classList.add("theme-" + cfg.theme); }
    if (cfg.blog_title) document.title = cfg.blog_title;
  } catch (_) {}

  var slug = new URLSearchParams(window.location.search).get("slug");
  if (!slug) { goHome(); return; }

  try {
    var result = await blog("get_post", { slug: slug });
    if (!result.found) {
      articleEl.innerHTML = '<a class="article-back" href="index.html">Back</a><div class="empty-msg"><h2>Post not found</h2></div>';
      wireBack(); return;
    }
    var p = result.post;
    document.title = p.title;
    var out = '<a class="article-back" href="index.html">Back</a>';
    if (p.image) out += '<img class="article-image" src="images/' + esc(p.image) + '" alt="">';
    out += '<h1 class="article-title">' + esc(p.title) + '</h1>';
    out += '<div class="article-meta">' + date(p._created_at);
    if (p.author_name) out += ' &middot; by ' + esc(p.author_name);
    out += '</div><div class="article-body">' + esc(p.body) + '</div>';
    articleEl.innerHTML = out;
    wireBack();
  } catch (err) {
    articleEl.innerHTML = '<a class="article-back" href="index.html">Back</a><div class="empty-msg"><p>' + esc(err.message) + '</p></div>';
    wireBack();
  }
})();
