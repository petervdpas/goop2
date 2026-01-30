// internal/ui/assets/js/goop-site.js
//
// Site file helpers for site pages.
// Usage:
//
//   <script src="/assets/js/goop-site.js"></script>
//
//   // list all files in the site directory
//   const files = await Goop.site.files();
//   // => [{Path, IsDir, Depth}, ...]
//
//   // read a file's contents as text
//   const html = await Goop.site.read("about.html");
//
//   // read as raw response (for binary, images, etc.)
//   const res = await Goop.site.fetch("photo.jpg");
//
(() => {
  window.Goop = window.Goop || {};

  window.Goop.site = {
    /** List all files in the peer's site directory. Returns [{Path, IsDir, Depth}] */
    files() {
      return fetch("/api/site/files").then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      });
    },

    /**
     * Read a site file as text.
     * Uses the self-proxy path: /p/{selfID}/{path}
     * Falls back to a direct relative fetch if identity is unavailable.
     */
    async read(path) {
      const url = await resolvePath(path);
      const res = await fetch(url);
      if (!res.ok) throw new Error(res.statusText);
      return res.text();
    },

    /**
     * Fetch a site file as a raw Response object.
     * Useful for images, binary, or when you need headers.
     */
    async fetch(path) {
      const url = await resolvePath(path);
      return fetch(url);
    },
  };

  async function resolvePath(path) {
    // If goop-identity is loaded, use the self-proxy path
    if (window.Goop.identity) {
      try {
        const id = await window.Goop.identity.id();
        return "/p/" + id + "/" + path.replace(/^\//, "");
      } catch (_) {}
    }
    // fallback: relative to current page
    return path;
  }
})();
