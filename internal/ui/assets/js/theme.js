(() => {
  const { onReady, qs, safeLocalStorageGet, safeLocalStorageSet } = window.Goop.core;

  const KEY = "goop.theme";
  const EVT = "goop:theme";

  function normalize(t) {
    return t === "light" || t === "dark" ? t : "dark";
  }

  function getBridge() {
    try {
      const qp = new URLSearchParams(location.search);
      const b = qp.get("bridge");
      return b ? String(b) : "";
    } catch {
      return "";
    }
  }

  async function postBridgeTheme(theme) {
    const bridge = getBridge();
    if (!bridge) return;

    try {
      await fetch(bridge.replace(/\/+$/, "") + "/theme", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ theme }),
      });
    } catch {
      // ignore: bridge is best-effort
    }
  }

  async function saveThemeToConfig(theme) {
    try {
      await Goop.api.settings.save({ theme });
    } catch {
      // ignore: best-effort
    }
  }

  function get() {
    // Layout.html already sets data-theme very early. Respect that first.
    const dom = document.documentElement.getAttribute("data-theme");
    if (dom === "light" || dom === "dark") return dom;

    return normalize(safeLocalStorageGet(KEY));
  }

  function set(t) {
    t = normalize(t);
    document.documentElement.setAttribute("data-theme", t);

    // Viewer origin localStorage (still useful for browser-opened viewer)
    safeLocalStorageSet(KEY, t);

    // Notify listeners in this origin
    window.dispatchEvent(new CustomEvent(EVT, { detail: { theme: t } }));

    // Save to peer config file (launcher theme is independent via data/ui.json)
    saveThemeToConfig(t);
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
