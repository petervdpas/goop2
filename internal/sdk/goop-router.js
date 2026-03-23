//
// Client-side page router for site templates.
// Routes between real HTML files using query parameters.
//
// Usage:
//
//   <script src="/sdk/goop-router.js"></script>
//
//   // Read a query parameter:
//   var slug = Goop.router.param('slug');
//   // URL: post.html?slug=hello-world → 'hello-world'
//
//   // Read all query parameters:
//   var params = Goop.router.params();
//   // URL: post.html?slug=hello&page=2 → {slug: 'hello', page: '2'}
//
//   // Navigate to a page within the same site:
//   Goop.router.go('post.html?slug=hello-world');
//
//   // Navigate back to the site index:
//   Goop.router.go('index.html');
//   // or just:
//   Goop.router.home();
//
//   // Get the current page name (without path prefix):
//   Goop.router.page();
//   // URL: /p/<peerID>/post.html → 'post.html'
//
// The router is peer-path aware: go() automatically prepends
// /p/<peerID>/ when viewing a remote peer's site.
//
(() => {
  window.Goop = window.Goop || {};

  let basePath = '';
  const m = window.location.pathname.match(/^(\/p\/[^/]+)\//);
  if (m) basePath = m[1];

  window.Goop.router = {
    param(name) {
      return new URLSearchParams(window.location.search).get(name);
    },

    params() {
      const out = {};
      new URLSearchParams(window.location.search).forEach((v, k) => { out[k] = v; });
      return out;
    },

    page() {
      let p = window.location.pathname;
      if (basePath && p.startsWith(basePath)) p = p.slice(basePath.length);
      p = p.replace(/^\/+/, '') || 'index.html';
      return p;
    },

    go(target) {
      const sep = target.startsWith('/') ? '' : '/';
      window.location.href = basePath + sep + target;
    },

    home() {
      window.location.href = basePath + '/';
    }
  };
})();
