(() => {
  window.Goop = window.Goop || {};

  let cached = null;

  async function load() {
    if (cached) return cached;
    const res = await fetch("/api/template/settings");
    if (!res.ok) throw new Error(res.statusText);
    cached = await res.json();
    return cached;
  }

  window.Goop.template = {
    get() {
      return load();
    },

    async requireEmail() {
      const s = await load();
      return !!s.require_email;
    },

    refresh() {
      cached = null;
    },
  };
})();
