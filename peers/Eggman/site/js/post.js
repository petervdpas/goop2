// Blog post detail page
(async function () {
  var db = Goop.data;
  var articleEl = document.getElementById("article");

  var basePath = '';
  var m = window.location.pathname.match(/^(\/p\/[^/]+)\//);
  if (m) basePath = m[1];

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  function goHome() {
    window.location.href = basePath + '/';
  }

  // ── Load blog config for theming ──
  try {
    var configRows = await db.query("blog_config", { limit: 100 });
    var html = document.documentElement;
    var accentToIdx = {
      "#b44d2d": "1", "#2d6a9f": "2", "#4a8f46": "3",
      "#7c4a9f": "4", "#c0882c": "5", "#2d7a6a": "6",
    };
    (configRows || []).forEach(function (r) {
      switch (r.key) {
        case "accent":
          if (r.value) {
            html.className = html.className.replace(/\baccent-\d+\b/g, "").trim();
            html.classList.add("accent-" + (accentToIdx[r.value] || "1"));
          }
          break;
        case "font":
          html.className = html.className.replace(/\bfont-\w+\b/g, "").trim();
          html.classList.add("font-" + (r.value || "serif"));
          break;
        case "theme":
          html.className = html.className.replace(/\btheme-\w+\b/g, "").trim();
          html.classList.add("theme-" + (r.value || "light"));
          break;
        case "blog_title":
          document.title = r.value || "Post";
          break;
      }
    });
  } catch (_) {}

  // ── Load the article ──
  var slug = new URLSearchParams(window.location.search).get("slug");
  if (!slug) {
    goHome();
    return;
  }

  try {
    var rows = await db.query("posts", {
      where: "slug = ? AND published = 1", args: [slug], limit: 1
    });
    if (!rows || rows.length === 0) {
      rows = await db.query("posts", {
        where: "_id = ? AND published = 1", args: [slug], limit: 1
      });
    }
    if (!rows || rows.length === 0) {
      articleEl.innerHTML =
        '<a class="article-back" href="index.html">Back</a>' +
        '<div class="empty-msg">' +
        '<h2>Post not found</h2>' +
        '<p class="loading">The article you are looking for does not exist.</p>' +
        '</div>';
      wireBack();
      return;
    }

    var p = rows[0];
    var date = p._created_at
      ? new Date(String(p._created_at).replace(" ", "T") + "Z")
          .toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })
      : "";

    document.title = p.title;

    var out = '<a class="article-back" href="index.html">Back</a>';
    if (p.image) {
      out += '<img class="article-image" src="images/' + esc(p.image) + '" alt="">';
    }
    out += '<h1 class="article-title">' + esc(p.title) + "</h1>";
    out += '<div class="article-meta">' + esc(date);
    if (p.author_name) {
      out += ' &middot; by ' + esc(p.author_name);
    }
    out += "</div>";
    out += '<div class="article-body">' + esc(p.body) + "</div>";
    articleEl.innerHTML = out;
    wireBack();
  } catch (err) {
    articleEl.innerHTML =
      '<a class="article-back" href="index.html">Back</a>' +
      '<div class="empty-msg"><p>Could not load article.</p><p class="loading">' +
      esc(err.message) + "</p></div>";
    wireBack();
  }

  function wireBack() {
    articleEl.querySelectorAll(".article-back").forEach(function (a) {
      a.addEventListener("click", function (e) {
        e.preventDefault();
        goHome();
      });
    });
  }
})();
