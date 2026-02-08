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
//   // upload a file (owner only — writes to content store)
//   await Goop.site.upload("images/photo.jpg", fileObj);
//
//   // delete a file (owner only)
//   await Goop.site.remove("images/photo.jpg");
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

    /**
     * Upload a file to the site content store.
     * @param {string} path - destination path (e.g., "images/photo.jpg")
     * @param {File|Blob} file - the file to upload
     * @returns {Promise<{status, path, etag}>}
     */
    async upload(path, file) {
      const fd = new FormData();
      fd.append("path", path);
      fd.append("file", file);
      const res = await fetch("/api/site/upload", { method: "POST", body: fd });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || res.statusText);
      }
      return res.json();
    },

    /**
     * Delete a file from the site content store.
     * @param {string} path - file path to delete
     * @returns {Promise<{status}>}
     */
    async remove(path) {
      const res = await fetch("/api/site/delete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: path }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || res.statusText);
      }
      return res.json();
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
