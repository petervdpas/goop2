// internal/ui/assets/js/20-theme.js
(() => {
  const { onReady, qs, safeLocalStorageGet, safeLocalStorageSet } = window.Goop.core;

  const KEY = "goop.theme";
  const EVT = "goop:theme";

  function normalize(t) {
    return t === "light" || t === "dark" ? t : "dark";
  }

  function get() {
    const dom = document.documentElement.getAttribute("data-theme");
    if (dom === "light" || dom === "dark") return dom;
    return normalize(safeLocalStorageGet(KEY));
  }

  function set(t) {
    t = normalize(t);
    document.documentElement.setAttribute("data-theme", t);
    safeLocalStorageSet(KEY, t);
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
