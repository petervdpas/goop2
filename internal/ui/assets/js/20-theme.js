(() => {
  const { onReady, qs, safeLocalStorageGet, safeLocalStorageSet } = window.Goop.core;

  const KEY = "goop.theme";
  const EVT = "goop:theme";

  function normalize(t) {
    return t === "light" || t === "dark" ? t : "dark";
  }

  function readParam(name) {
    try {
      return new URL(window.location.href).searchParams.get(name);
    } catch {
      return null;
    }
  }

  function writeParam(name, value) {
    try {
      const u = new URL(window.location.href);
      if (value == null) u.searchParams.delete(name);
      else u.searchParams.set(name, value);
      window.history.replaceState({}, "", u.toString());
    } catch {}
  }

  function bridgeBase() {
    const b = readParam("bridge");
    if (!b) return null;
    // trim trailing slashes
    return b.replace(/\/+$/, "");
  }

  async function publishToBridge(theme) {
    const b = bridgeBase();
    if (!b) return;

    try {
      await fetch(b + "/theme", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ theme }),
      });
    } catch {
      // ignore: viewer still works even if bridge unreachable
    }
  }

  function get() {
    // 1) DOM
    const dom = document.documentElement.getAttribute("data-theme");
    if (dom === "light" || dom === "dark") return dom;

    // 2) URL handoff wins (launcher -> viewer)
    const urlT = readParam("theme");
    if (urlT === "light" || urlT === "dark") return urlT;

    // 3) localStorage for this origin (useful within this viewer session)
    return normalize(safeLocalStorageGet(KEY));
  }

  function set(t) {
    t = normalize(t);
    document.documentElement.setAttribute("data-theme", t);

    // persist for this viewer origin (session/navigation)
    safeLocalStorageSet(KEY, t);

    // keep URL stable for refresh/copy/paste
    writeParam("theme", t);

    // push to Wails so launcher can match
    publishToBridge(t);

    window.dispatchEvent(new CustomEvent(EVT, { detail: { theme: t } }));
  }

  function initToggle() {
    const toggle = qs("#themeToggle");
    if (!toggle) return;

    toggle.checked = (get() === "light");

    toggle.addEventListener("change", () => {
      set(toggle.checked ? "light" : "dark");
    });

    window.addEventListener(EVT, (e) => {
      const t = e?.detail?.theme === "light" ? "light" : "dark";
      toggle.checked = (t === "light");
    });
  }

  window.Goop.theme = { get, set, EVT, initToggle };

  onReady(() => initToggle());
})();
